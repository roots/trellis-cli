package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/fatih/color"
	"github.com/hashicorp/cli"
	"github.com/manifoldco/promptui"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/pkg/server"
	"github.com/roots/trellis-cli/trellis"
)

func NewServerDnsCommand(ui cli.Ui, trellis *trellis.Trellis) *ServerDnsCommand {
	c := &ServerDnsCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

type ServerDnsCommand struct {
	UI           cli.Ui
	Trellis      *trellis.Trellis
	flags        *flag.FlagSet
	providerFlag string
	force        bool
	ip           string
}

func (c *ServerDnsCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.providerFlag, "provider", "", "Cloud provider (digitalocean, hetzner)")
	c.flags.BoolVar(&c.force, "force", false, "Force update of DNS records even if they exist")
	c.flags.StringVar(&c.ip, "ip", "", "Host IP of DNS records")
}

func (c *ServerDnsCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 1, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	environment := args[0]

	environmentErr := c.Trellis.ValidateEnvironment(environment)
	if environmentErr != nil {
		c.UI.Error(environmentErr.Error())
		return 1
	}

	if environment == "development" {
		c.UI.Error("dns command only supports non-development environments")
		return 1
	}

	providerName := c.resolveProvider()

	token, err := server.GetProviderToken(providerName, c.UI)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error: %s API token is required.", providerName))
		return 1
	}

	provider, err := server.NewProviderWithDNS(providerName, token)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	ctx := context.Background()

	if c.ip == "" {
		c.ip, err = c.selectIP(ctx, provider)
		c.UI.Info("")

		if err != nil {
			c.UI.Error("Error: can't continue without a host IP.")
			return 1
		}
	}

	c.UI.Info(fmt.Sprintf("DNS records for the following domains will be pointed to %s", c.ip))

	hostsByDomain := c.Trellis.Environments[environment].AllHostsByDomain()
	for _, hosts := range hostsByDomain {
		for _, host := range hosts {
			c.UI.Info("  " + host.Fqdn)
		}
	}

	c.UI.Info("")

	prompt := promptui.Prompt{Label: "Create DNS records", IsConfirm: true}
	if _, err = prompt.Run(); err != nil {
		return 0
	}

	for domain, hosts := range hostsByDomain {
		// Check if zone exists, create if not
		_, exists, err := provider.GetZone(ctx, domain)
		if err != nil {
			c.UI.Error(fmt.Sprintf("Error checking domain %s: %v", domain, err))
			return 1
		}

		if !exists {
			if err := provider.CreateZone(ctx, domain); err != nil {
				c.UI.Error(fmt.Sprintf("Error: could not create domain %s\n%v", domain, err))
				return 1
			}
		}

		// Get existing records
		existingRecords, err := provider.ListRecords(ctx, domain)
		if err != nil {
			c.UI.Error(fmt.Sprintf("Error listing records for %s: %v", domain, err))
			return 1
		}

		for _, host := range hosts {
			// Check if record exists
			var existingRecord *server.DNSRecord
			for _, r := range existingRecords {
				if r.Type == "A" && r.Name == host.Name {
					existingRecord = &r
					break
				}
			}

			if existingRecord != nil {
				if c.force {
					err := provider.DeleteRecord(ctx, domain, existingRecord.ID)
					if err != nil {
						c.UI.Error(fmt.Sprintf("Error: could not delete existing record %s\n%v", host.Fqdn, err))
						continue
					}
				} else {
					c.UI.Info(fmt.Sprintf("%s %s", color.YellowString("[SKIPPED]"), host.Fqdn))
					continue
				}
			}

			_, err := provider.CreateRecord(ctx, domain, server.DNSRecord{
				Type:  "A",
				Name:  host.Name,
				Value: c.ip,
			})

			if err == nil {
				c.UI.Info(fmt.Sprintf("%s %s", color.GreenString("[CREATED]"), host.Fqdn))
			} else {
				c.UI.Info(fmt.Sprintf("%s %s", color.RedString("[ERROR]"), host.Fqdn))
				c.UI.Error(err.Error())
			}
		}
	}

	return 0
}

func (c *ServerDnsCommand) resolveProvider() server.ProviderName {
	if c.providerFlag != "" {
		return server.ProviderName(c.providerFlag)
	}
	if env := os.Getenv("TRELLIS_SERVER_PROVIDER"); env != "" {
		return server.ProviderName(env)
	}
	if c.Trellis.CliConfig.Server.Provider != "" {
		return server.ProviderName(c.Trellis.CliConfig.Server.Provider)
	}
	return server.ProviderDigitalOcean
}

func (c *ServerDnsCommand) Synopsis() string {
	return "Creates DNS records for all WordPress sites' hosts in an environment"
}

func (c *ServerDnsCommand) Help() string {
	helpText := `
Usage: trellis server dns [options] ENVIRONMENT

Creates DNS records for all WordPress sites' hosts in an environment.
DNS records (type A) will be created for each host that all point to the
server IP; the host IP can be manually overridden if need be.

Supported providers:
  - digitalocean (default)
  - hetzner

The provider can be configured via:
  1. --provider flag
  2. TRELLIS_SERVER_PROVIDER environment variable
  3. server.provider in trellis.cli.yml

Note: this command assumes your domain's DNS is managed by the cloud provider
and the nameservers have already been set appropriately.

This command only supports Trellis' standard setup of one server per environment.
If your sites are split across multiple servers, then this command won't work and
you should manage your DNS manually.

Create DNS records for the production server:

  $ trellis server dns production

Force re-creation of existing DNS records:

  $ trellis server dns --force production

Manually specify the host IP to use:

  $ trellis server dns --ip 1.2.3.4 production

Arguments:
  ENVIRONMENT Name of environment (ie: production)

Options:
      --provider  Cloud provider (digitalocean, hetzner)
      --force     Force updating DNS records even if they already exist
      --ip        Host IP of DNS records
  -h, --help      Show this help
`

	return strings.TrimSpace(helpText)
}

func (c *ServerDnsCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.PredictEnvironment(c.flags)
}

func (c *ServerDnsCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--provider": complete.PredictSet("digitalocean", "hetzner"),
		"--ip":       complete.PredictNothing,
		"--force":    complete.PredictNothing,
	}
}

func (c *ServerDnsCommand) selectIP(ctx context.Context, provider server.Provider) (string, error) {
	servers, err := provider.GetServers(ctx)
	if err != nil {
		return "", err
	}

	if len(servers) == 0 {
		return "", fmt.Errorf("no servers found")
	}

	tpl := `{{.Name}} [{{.PublicIPv4 | faint}}]`

	templates := &promptui.SelectTemplates{
		Active:   fmt.Sprintf("%s %s", promptui.IconSelect, tpl),
		Inactive: tpl,
		Selected: fmt.Sprintf(`{{ "%s" | green }} %s`, promptui.IconGood, tpl),
		FuncMap: template.FuncMap{
			"green": promptui.Styler(promptui.FGGreen),
			"faint": promptui.Styler(promptui.FGFaint),
		},
	}

	prompt := promptui.Select{
		Label:     "Select Server IP",
		Templates: templates,
		Items:     servers,
		Size:      len(servers),
	}

	i, _, err := prompt.Run()
	if err != nil {
		return "", err
	}

	return servers[i].PublicIPv4, nil
}
