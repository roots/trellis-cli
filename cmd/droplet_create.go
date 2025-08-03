package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/godo/util"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/hashicorp/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/digitalocean"
	"github.com/roots/trellis-cli/trellis"
	"golang.org/x/crypto/ssh"
)

var defaultSshKeys = []string{"~/.ssh/id_ed25519.pub", "~/.ssh/id_rsa.pub"}

func NewDropletCreateCommand(ui cli.Ui, trellis *trellis.Trellis) *DropletCreateCommand {
	c := &DropletCreateCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

type DropletCreateCommand struct {
	UI            cli.Ui
	Trellis       *trellis.Trellis
	doClient      *digitalocean.Client
	flags         *flag.FlagSet
	sshKey        string
	region        string
	image         string
	size          string
	skipProvision bool
}

func (c *DropletCreateCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.sshKey, "ssh-key", "", "Path to SSH public key to automatically add to new server")
	c.flags.StringVar(&c.region, "region", "", "Region to create the server in")
	c.flags.StringVar(&c.image, "image", "ubuntu-20-04-x64", "Server image")
	c.flags.StringVar(&c.size, "size", "", "Server size/type to create")
	c.flags.BoolVar(&c.skipProvision, "skip-provision", false, "Create the server but skip provisioning")
}

func (c *DropletCreateCommand) Run(args []string) int {
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
		c.UI.Error("create command only supports staging/production environments")
		return 1
	}

	accessToken, err := digitalocean.GetAccessToken(c.UI)
	if err != nil {
		c.UI.Error("Error: DigitalOcean access token is required.")
		return 1
	}

	c.doClient = digitalocean.NewClient(accessToken)

	sshKeys := defaultSshKeys
	if c.sshKey != "" {
		sshKeys = []string{c.sshKey}
	}

	sshKeyPath, contents, publicKey, err := digitalocean.LoadSSHKey(sshKeys)
	if err == nil {
		err = c.checkSSHKey(sshKeyPath, contents, publicKey)
	}

	if err != nil {
		c.UI.Error("Error: can't continue without an SSH key")
		c.UI.Error(err.Error())
		c.UI.Error("\nThe --ssh-key option can be used to specify the path of a valid SSH key.")
		return 1
	}

	c.UI.Info(fmt.Sprintf("Using SSH key at %s\n", sshKeyPath))

	var region *godo.Region

	if c.region == "" {
		region, err = c.selectRegion()

		if err != nil {
			c.UI.Error(err.Error())
			return 1
		}

		c.region = region.Slug
	}

	if c.size == "" {
		c.size, err = c.selectSize(region)

		if err != nil {
			c.UI.Error(err.Error())
			return 1
		}
	}

	siteNames := c.Trellis.SiteNamesFromEnvironment(environment)
	name, err := c.askDropletName(siteNames[0])
	if err != nil {
		return 1
	}

	droplet, err := c.createDroplet(c.region, c.size, c.image, publicKey, name, environment)
	if err != nil {
		return 1
	}

	droplet, err = c.waitForSSH(droplet)
	if err != nil {
		return 1
	}

	ip, _ := droplet.PublicIPv4()

	_, err = c.Trellis.UpdateHosts(environment, ip)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error updating Trellis hosts file: %s", err))
		return 1
	}

	c.UI.Info(fmt.Sprintf("%s Updated hosts/%s with droplet IP: %s", color.GreenString("[✓]"), environment, ip))

	if c.skipProvision {
		c.UI.Warn(fmt.Sprintf("Skipping provision. Run `trellis provision %s` to manually provision.", environment))
	} else {
		c.UI.Info("\nProvisioning server...\n")

		provisionCmd := NewProvisionCommand(c.UI, c.Trellis)
		return provisionCmd.Run([]string{environment})
	}

	return 0
}

func (c *DropletCreateCommand) Synopsis() string {
	return "Creates a DigitalOcean Droplet server and provisions it"
}

func (c *DropletCreateCommand) Help() string {
	helpText := `
Usage: trellis droplet create [options] ENVIRONMENT

Creates a droplet (server) on DigitalOcean for the environment specified.

Only remote servers (for staging and production) are currently supported.
Development should be managed separately through Vagrant.

This command requires a DigitalOcean personal access token.
Link: https://cloud.digitalocean.com/account/api/tokens/new

If the DIGITALOCEAN_ACCESS_TOKEN environment variable is not set, the command
will prompt for one.

Create a production server (region and size will be prompted):

  $ trellis droplet create production

Create a 1gb server in the nyc3 region:

  $ trellis droplet create --region=nyc3 --size=s-1vcpu-1gb production

Create a 1gb server with a specific Ubuntu image:

  $ trellis droplet create --region=nyc3 --image=ubuntu-18-04-x64 --size=s-1vcpu-1gb production

Create a server but skip provisioning:

  $ trellis droplet create --skip-provision production

Arguments:
  ENVIRONMENT Name of environment (ie: production)

Options:
      --region          Region to create the server in
      --image           (default: ubuntu-20-04-x64) Server image (ie: Linux distribution)
      --size            Server size/type
      --skip-provision  Skip provision after server is created
      --ssh-key         (default: ~/.ssh/id_rsa.pub or ~/.ssh/id_ed25519.pub) path to SSH public key to be added on the server
  -h, --help            show this help
`

	return strings.TrimSpace(helpText)
}

func (c *DropletCreateCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.PredictEnvironment(c.flags)
}

func (c *DropletCreateCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--region":          complete.PredictNothing,
		"--size":            complete.PredictNothing,
		"--skip--provision": complete.PredictNothing,
		"--ssh-key":         complete.PredictFiles("*.pub"),
	}
}

func (c *DropletCreateCommand) askDropletName(siteName string) (name string, err error) {
	name, err = c.UI.Ask(fmt.Sprintf("Droplet name [%s]:", color.GreenString(siteName)))
	if err != nil {
		return "", err
	}

	if name == "" {
		name = siteName
	}

	return name, nil
}

func (c *DropletCreateCommand) createDroplet(region string, size string, image string, publicKey ssh.PublicKey, name string, env string) (droplet *godo.Droplet, err error) {
	droplet, monitorUri, err := c.doClient.CreateDroplet(region, size, image, publicKey, name, env)
	if err != nil {
		return nil, err
	}

	c.UI.Info(fmt.Sprintf("\n%s Server created => https://cloud.digitalocean.com/droplets/%d", color.GreenString("[✓]"), droplet.ID))

	s := NewSpinner(
		SpinnerCfg{
			Message:     "Waiting for server to boot (this may take a minute)",
			StopMessage: "Server booted",
			FailMessage: "Server did not become active (or timed out)",
		},
	)

	s.Start()
	err = util.WaitForActive(context.TODO(), c.doClient.Client, monitorUri)

	if err != nil {
		s.StopFail()
		c.UI.Error(err.Error())
		return nil, err
	}

	s.Stop()

	return droplet, nil
}

func (c *DropletCreateCommand) checkSSHKey(path string, contents []byte, publicKey ssh.PublicKey) error {
	response, err := c.doClient.GetSSHKey(publicKey)

	switch response.StatusCode {
	case 404:
		c.UI.Info(fmt.Sprintf("SSH Key [%s] does not exist in DigitalOcean.", path))

		prompt := promptui.Prompt{
			Label:     "Add SSH key to account",
			IsConfirm: true,
		}

		_, err := prompt.Run()

		if err != nil {
			return errors.New("Can't continue without an SSH key on your account.")
		}

		return c.doClient.CreateSSHKey(string(contents))
	case 200:
		return nil
	default:
		return fmt.Errorf("Could not create SSH key on DigitalOcean: %v", err)
	}
}

func (c *DropletCreateCommand) selectRegion() (region *godo.Region, err error) {
	availableRegions, err := c.doClient.GetAvailableRegions()
	if err != nil {
		return nil, err
	}

	tpl := `{{ .Name }} [{{ .Slug | faint}}]`

	templates := &promptui.SelectTemplates{
		Active:   fmt.Sprintf("%s %s", promptui.IconSelect, tpl),
		Inactive: tpl,
		Selected: fmt.Sprintf(`{{ "%s" | green }} %s`, promptui.IconGood, tpl),
	}

	prompt := promptui.Select{
		Label:     "Select Region",
		Templates: templates,
		Items:     availableRegions,
		Size:      len(availableRegions),
	}

	i, _, err := prompt.Run()

	if err != nil {
		return nil, err
	}

	return &availableRegions[i], nil
}

func (c *DropletCreateCommand) selectSize(region *godo.Region) (size string, err error) {
	sizes, err := c.doClient.GetSizesByRegion(region)
	if err != nil {
		return "", err
	}

	tpl := `${{ .PriceMonthly }}/mo - {{ .Slug | faint }} [{{ .Memory }}MB | {{ .Vcpus }} CPUs | {{ .Disk }}GB SSD disk | {{ .Transfer }} TB transfer]`

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

func (c *DropletCreateCommand) waitForSSH(droplet *godo.Droplet) (*godo.Droplet, error) {
	droplet, ip, err := c.doClient.GetDroplet(droplet)
	if err != nil {
		return droplet, err
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Minute,
	)
	defer cancel()

	s := NewSpinner(
		SpinnerCfg{
			Message:     "Waiting for SSH (this may take a minute)",
			StopMessage: "SSH available",
			FailMessage: "Timeout waiting for SSH",
		},
	)
	s.Start()
	err = digitalocean.CheckSSH(ip, ctx)

	if err != nil {
		s.StopFail()
	}
	s.Stop()

	return droplet, nil
}
