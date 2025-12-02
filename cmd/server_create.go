package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/hashicorp/cli"
	"github.com/manifoldco/promptui"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/pkg/server"
	"github.com/roots/trellis-cli/trellis"
)

func NewServerCreateCommand(ui cli.Ui, trellis *trellis.Trellis) *ServerCreateCommand {
	c := &ServerCreateCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

type ServerCreateCommand struct {
	UI            cli.Ui
	Trellis       *trellis.Trellis
	flags         *flag.FlagSet
	providerFlag  string
	sshKey        string
	region        string
	image         string
	size          string
	skipProvision bool
}

func (c *ServerCreateCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.providerFlag, "provider", "", "Cloud provider (digitalocean, hetzner)")
	c.flags.StringVar(&c.sshKey, "ssh-key", "", "Path to SSH public key to automatically add to new server")
	c.flags.StringVar(&c.region, "region", "", "Region to create the server in")
	c.flags.StringVar(&c.image, "image", "", "Server image (default: Ubuntu 24.04)")
	c.flags.StringVar(&c.size, "size", "", "Server size/type to create")
	c.flags.BoolVar(&c.skipProvision, "skip-provision", false, "Create the server but skip provisioning")
}

func (c *ServerCreateCommand) Run(args []string) int {
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
		c.UI.Error("server create command only supports staging/production environments")
		return 1
	}

	providerName := c.resolveProvider()

	token, err := server.GetProviderToken(providerName, c.UI)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error: %s API token is required.", providerName))
		return 1
	}

	provider, err := server.NewProvider(providerName, token)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Info(fmt.Sprintf("Using %s provider\n", provider.DisplayName()))

	// SSH key handling
	sshKeyPaths := server.DefaultSSHKeyPaths
	if c.sshKey != "" {
		sshKeyPaths = []string{c.sshKey}
	}

	sshKeyPath, contents, publicKey, err := server.LoadSSHKey(sshKeyPaths)
	if err != nil {
		c.UI.Error("Error: can't continue without an SSH key")
		c.UI.Error(err.Error())
		c.UI.Error("\nThe --ssh-key option can be used to specify the path of a valid SSH key.")
		return 1
	}

	ctx := context.Background()
	fingerprint := server.SSHKeyFingerprint(publicKey)

	existingKey, err := provider.GetSSHKey(ctx, fingerprint)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error checking SSH key: %s", err))
		return 1
	}

	if existingKey == nil {
		c.UI.Info(fmt.Sprintf("SSH Key [%s] does not exist in %s.", sshKeyPath, provider.DisplayName()))

		prompt := promptui.Prompt{
			Label:     "Add SSH key to account",
			IsConfirm: true,
		}

		_, err := prompt.Run()
		if err != nil {
			c.UI.Error("Can't continue without an SSH key on your account.")
			return 1
		}

		keyName := "trellis-cli-ssh-key"
		if u, err := user.Current(); err == nil {
			keyName = u.Username
		}

		_, err = provider.CreateSSHKey(ctx, keyName, string(contents))
		if err != nil {
			c.UI.Error(fmt.Sprintf("Error creating SSH key: %s", err))
			return 1
		}
	}

	c.UI.Info(fmt.Sprintf("Using SSH key at %s\n", sshKeyPath))

	// Region selection
	if c.region == "" {
		regions, err := provider.GetRegions(ctx)
		if err != nil {
			c.UI.Error(fmt.Sprintf("Error fetching regions: %s", err))
			return 1
		}

		c.region, err = c.selectRegion(regions)
		if err != nil {
			c.UI.Error(err.Error())
			return 1
		}
	}

	// Size selection
	if c.size == "" {
		sizes, err := provider.GetSizes(ctx, c.region)
		if err != nil {
			c.UI.Error(fmt.Sprintf("Error fetching sizes: %s", err))
			return 1
		}

		c.size, err = c.selectSize(sizes)
		if err != nil {
			c.UI.Error(err.Error())
			return 1
		}
	}

	// Image default
	if c.image == "" {
		c.image = server.DefaultImage(providerName)
	}

	// Server name
	siteNames := c.Trellis.SiteNamesFromEnvironment(environment)
	name, err := c.askServerName(siteNames[0])
	if err != nil {
		return 1
	}

	// Create server
	srv, err := c.createServer(ctx, provider, name, environment, fingerprint)
	if err != nil {
		return 1
	}

	// Wait for server to be ready
	srv, err = c.waitForServer(ctx, provider, srv)
	if err != nil {
		return 1
	}

	// Update hosts file
	_, err = c.Trellis.UpdateHosts(environment, srv.PublicIPv4)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error updating Trellis hosts file: %s", err))
		return 1
	}

	c.UI.Info(fmt.Sprintf("%s Updated hosts/%s with server IP: %s", color.GreenString("[✓]"), environment, srv.PublicIPv4))

	if c.skipProvision {
		c.UI.Warn(fmt.Sprintf("Skipping provision. Run `trellis provision %s` to manually provision.", environment))
	} else {
		c.UI.Info("\nProvisioning server...\n")

		provisionCmd := NewProvisionCommand(c.UI, c.Trellis)
		return provisionCmd.Run([]string{environment})
	}

	return 0
}

func (c *ServerCreateCommand) resolveProvider() server.ProviderName {
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

func (c *ServerCreateCommand) Synopsis() string {
	return "Creates a cloud server and provisions it"
}

func (c *ServerCreateCommand) Help() string {
	helpText := `
Usage: trellis server create [options] ENVIRONMENT

Creates a server on a cloud provider for the environment specified.

Only remote servers (for staging and production) are currently supported.
Development should be managed separately through the VM commands.

Supported providers:
  - digitalocean (default)
  - hetzner

The provider can be configured via:
  1. --provider flag
  2. TRELLIS_SERVER_PROVIDER environment variable
  3. server.provider in trellis.cli.yml

Create a production server (region and size will be prompted):

  $ trellis server create production

Create a server with Hetzner:

  $ trellis server create --provider=hetzner production

Create a server in a specific region:

  $ trellis server create --region=nyc3 --size=s-1vcpu-1gb production

Create a server but skip provisioning:

  $ trellis server create --skip-provision production

Arguments:
  ENVIRONMENT Name of environment (ie: production)

Options:
      --provider        Cloud provider (digitalocean, hetzner)
      --region          Region to create the server in
      --image           Server image (default: Ubuntu 24.04)
      --size            Server size/type
      --skip-provision  Skip provision after server is created
      --ssh-key         Path to SSH public key to be added on the server
  -h, --help            show this help
`

	return strings.TrimSpace(helpText)
}

func (c *ServerCreateCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.PredictEnvironment(c.flags)
}

func (c *ServerCreateCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--provider":        complete.PredictSet("digitalocean", "hetzner"),
		"--region":          complete.PredictNothing,
		"--size":            complete.PredictNothing,
		"--skip--provision": complete.PredictNothing,
		"--ssh-key":         complete.PredictFiles("*.pub"),
	}
}

func (c *ServerCreateCommand) askServerName(siteName string) (string, error) {
	name, err := c.UI.Ask(fmt.Sprintf("Server name [%s]:", color.GreenString(siteName)))
	if err != nil {
		return "", err
	}

	if name == "" {
		name = siteName
	}

	return name, nil
}

func (c *ServerCreateCommand) selectRegion(regions []server.Region) (string, error) {
	tpl := `{{ .Name }} [{{ .Slug | faint}}]`

	templates := &promptui.SelectTemplates{
		Active:   fmt.Sprintf("%s %s", promptui.IconSelect, tpl),
		Inactive: tpl,
		Selected: fmt.Sprintf(`{{ "%s" | green }} %s`, promptui.IconGood, tpl),
	}

	prompt := promptui.Select{
		Label:     "Select Region",
		Templates: templates,
		Items:     regions,
		Size:      len(regions),
	}

	i, _, err := prompt.Run()
	if err != nil {
		return "", err
	}

	return regions[i].Slug, nil
}

func (c *ServerCreateCommand) selectSize(sizes []server.Size) (string, error) {
	tpl := `${{ printf "%.2f" .PriceMonthly }}/mo - {{ .Slug | faint }} [{{ .Memory }}MB | {{ .VCPUs }} CPUs | {{ .Disk }}GB SSD]`

	templates := &promptui.SelectTemplates{
		Active:   fmt.Sprintf("%s %s", promptui.IconSelect, tpl),
		Inactive: tpl,
		Selected: fmt.Sprintf(`{{ "%s" | green }} %s`, promptui.IconGood, tpl),
	}

	prompt := promptui.Select{
		Label:     "Select Size",
		Items:     sizes,
		Templates: templates,
		Size:      len(sizes),
	}

	i, _, err := prompt.Run()
	if err != nil {
		return "", err
	}

	return sizes[i].Slug, nil
}

func (c *ServerCreateCommand) createServer(ctx context.Context, provider server.Provider, name, env, sshFingerprint string) (*server.Server, error) {
	srv, err := provider.CreateServer(ctx, server.CreateServerOptions{
		Name:      name,
		Region:    c.region,
		Size:      c.size,
		Image:     c.image,
		SSHKeyIDs: []string{sshFingerprint},
		Tags:      map[string]string{"env": env},
	})
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error creating server: %s", err))
		return nil, err
	}

	c.UI.Info(fmt.Sprintf("\n%s Server created => %s", color.GreenString("[✓]"), srv.DashboardURL))

	return srv, nil
}

func (c *ServerCreateCommand) waitForServer(ctx context.Context, provider server.Provider, srv *server.Server) (*server.Server, error) {
	s := NewSpinner(
		SpinnerCfg{
			Message:     "Waiting for server to boot (this may take a minute)",
			StopMessage: "Server booted",
			FailMessage: "Server did not become active (or timed out)",
		},
	)

	s.Start()
	srv, err := provider.WaitForServer(ctx, srv.ID, 5*time.Minute)
	if err != nil {
		s.StopFail()
		c.UI.Error(err.Error())
		return nil, err
	}
	s.Stop()

	// Wait for SSH
	s = NewSpinner(
		SpinnerCfg{
			Message:     "Waiting for SSH (this may take a minute)",
			StopMessage: "SSH available",
			FailMessage: "Timeout waiting for SSH",
		},
	)

	sshCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	s.Start()
	err = server.WaitForSSH(sshCtx, srv.PublicIPv4)
	if err != nil {
		s.StopFail()
		return nil, err
	}
	s.Stop()

	return srv, nil
}
