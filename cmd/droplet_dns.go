package cmd

import (
	"flag"
	"fmt"
	"strings"
	"text/template"

	"github.com/digitalocean/godo"
	"github.com/fatih/color"
	"github.com/hashicorp/cli"
	"github.com/manifoldco/promptui"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/digitalocean"
	"github.com/roots/trellis-cli/trellis"
)

func NewDropletDnsCommand(ui cli.Ui, trellis *trellis.Trellis) *DropletDnsCommand {
	c := &DropletDnsCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

type DropletDnsCommand struct {
	UI       cli.Ui
	Trellis  *trellis.Trellis
	doClient *digitalocean.Client
	flags    *flag.FlagSet
	force    bool
	ip       string
}

func (c *DropletDnsCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.force, "force", false, "Force update of DNS records even if they exist")
	c.flags.StringVar(&c.ip, "ip", "", "Host IP of DNS records")
}

func (c *DropletDnsCommand) Run(args []string) int {
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

	accessToken, err := digitalocean.GetAccessToken(c.UI)
	if err != nil {
		c.UI.Error("Error: DigitalOcean access token is required.")
		return 1
	}

	c.doClient = digitalocean.NewClient(accessToken)

	if c.ip == "" {
		c.ip, err = c.selectIP()
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

	hostRecords := c.doClient.GetHostRecords(hostsByDomain)

	for _, host := range hostRecords {
		if !host.Domain.Exists {
			if _, err := c.doClient.CreateDomain(host.Domain.Name); err != nil {
				c.UI.Error(fmt.Sprintf("Error: could not create domain %s\n%v", host.Domain.Name, err))
				return 1
			}
		}

		if host.Record != nil {
			if c.force {
				err := c.doClient.DeleteDomainRecord(*host.Record, host.Domain.Name)

				if err != nil {
					c.UI.Error(fmt.Sprintf("Error: could not delete existing record %s\n%v", host.Fqdn, err))
					continue
				}
			} else {
				c.UI.Info(fmt.Sprintf("%s %s", color.YellowString("[SKIPPED]"), host.Fqdn))
				continue
			}
		}

		_, err := c.doClient.CreateDomainRecord(host.Domain.Name, host.Name, c.ip)

		if err == nil {
			c.UI.Info(fmt.Sprintf("%s %s", color.GreenString("[CREATED]"), host.Fqdn))
		} else {
			c.UI.Info(fmt.Sprintf("%s %s\n", color.RedString("[ERROR]"), host.Fqdn))
			c.UI.Error(err.Error())
		}
	}

	return 0
}

func (c *DropletDnsCommand) Synopsis() string {
	return "Creates DNS records for all WordPress sites' hosts in an environment"
}

func (c *DropletDnsCommand) Help() string {
	helpText := `
Usage: trellis droplet dns [options] ENVIRONMENT

Creates DNS records for all WordPress sites' hosts in an environment.
DNS records (type A) will be created for each host that all point to the
server IP found in the environment's hosts file (eg: 'hosts/production');
the host IP can be manually overriden if need be.

-------------------------------------------------------------------------------
Note: this command assumes your domain's DNS is managed by DigitalOcean and the
nameservers have already been set to DigitalOcean's.

This command only supports Trellis' standard setup of one server per environment.
If your sites are split across multiple servers, then this command won't work and
you should manage your DNS manually.
-------------------------------------------------------------------------------

Using this wordpress_sites.yml config as an example:

  wordpress_sites:
    site1.com:
      site_hosts:
        - canonical: site1.com
          redirects:
            - www.site1.com
    site2.com:
      site_hosts:
        - canonical: site2.com
          redirects:
            - www.site2.com
        - canonical: different-site.com
          redirects:
            - www.different-site.com

The following hosts will have DNS A records created pointing to the production host IP:
  - site1.com
  - www.site1.com
  - site2.com
  - www.site2.com
  - different-site.com
  - www.different-site.com

Create DNS records for the production droplet:

  $ trellis droplet dns production

Force re-creation of existing DNS records:

  $ trellis droplet dns --force production

Manually specify the host IP to use:

  $ trellis droplet dns --ip 1.2.3.4 production

Arguments:
  ENVIRONMENT Name of environment (ie: production)

Options:
  --force     Force updating DNS records even if they already exist
  --ip        Host IP of DNS records
  -h, --help  Show this help
`

	return strings.TrimSpace(helpText)
}

func (c *DropletDnsCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.PredictEnvironment(c.flags)
}

func (c *DropletDnsCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--ip":    complete.PredictNothing,
		"--force": complete.PredictNothing,
	}
}

func (c *DropletDnsCommand) selectIP() (ip string, err error) {
	droplets, err := c.doClient.GetDroplets()
	if err != nil {
		return "", err
	}

	tpl := `{{.Name}} [{{PublicIP . | faint}}]`

	templates := &promptui.SelectTemplates{
		Active:   fmt.Sprintf("%s %s", promptui.IconSelect, tpl),
		Inactive: tpl,
		Selected: fmt.Sprintf(`{{ "%s" | green }} %s`, promptui.IconGood, tpl),
		FuncMap: template.FuncMap{
			"green": promptui.Styler(promptui.FGGreen),
			"faint": promptui.Styler(promptui.FGFaint),
			"PublicIP": func(droplet godo.Droplet) string {
				ip, _ := droplet.PublicIPv4()
				return ip
			},
		},
	}

	prompt := promptui.Select{
		Label:     "Select Droplet IP",
		Templates: templates,
		Items:     droplets,
		Size:      len(droplets),
	}

	i, _, err := prompt.Run()

	if err != nil {
		return "", err
	}

	ip, err = droplets[i].PublicIPv4()
	return ip, err
}
