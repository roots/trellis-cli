package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/digitalocean/godo"
	"github.com/digitalocean/godo/util"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/mitchellh/cli"
	"github.com/mitchellh/go-homedir"
	"github.com/posener/complete"
	"golang.org/x/crypto/ssh"
	"github.com/roots/trellis-cli/digitalocean"
	"github.com/roots/trellis-cli/trellis"
)

var client *digitalocean.Client

func NewDropletCreateCommand(ui cli.Ui, trellis *trellis.Trellis) *DropletCreateCommand {
	c := &DropletCreateCommand{UI: ui, Trellis: trellis, playbook: &Playbook{ui: ui}}
	c.init()
	return c
}

type DropletCreateCommand struct {
	UI            cli.Ui
	Trellis       *trellis.Trellis
	flags         *flag.FlagSet
	sshKey        string
	region        string
	size          string
	skipProvision bool
	playbook      PlaybookRunner
}

func (c *DropletCreateCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.sshKey, "ssh-key", "~/.ssh/id_rsa.pub", "Path to SSH public key to automatically add to new server")
	c.flags.StringVar(&c.region, "region", "", "Region to create the server in")
	c.flags.StringVar(&c.size, "size", "", "Server size/type to create")
	c.flags.BoolVar(&c.skipProvision, "skip-provision", false, "Create the server but skip provisioning")
}

func (c *DropletCreateCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

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

	accessToken, err := getAccessToken(c.UI)
	if err != nil {
		c.UI.Error("Error: DIGITALOCEAN_ACCESS_TOKEN is required.")
		return 1
	}

	client = digitalocean.NewClient(accessToken)

	if c.sshKey == "" {
		c.UI.Error("Error: --ssh-key option is empty")
		return 1
	}

	keyString, publicKey, err := loadSSHKey(c.sshKey)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error: no valid SSH public key found at %s", c.sshKey))
		return 1
	}

	err = checkSSHKey(c.UI, keyString, publicKey)

	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	var region *godo.Region

	if c.region == "" {
		region, err = selectRegion()

		if err != nil {
			c.UI.Error(err.Error())
			return 1
		}

		c.region = region.Slug
	}

	if c.size == "" {
		c.size, err = selectSize(region)

		if err != nil {
			c.UI.Error(err.Error())
			return 1
		}
	}

	siteNames := c.Trellis.SiteNamesFromEnvironment(environment)
	name, err := askDropletName(c.UI, siteNames[0])
	if err != nil {
		return 1
	}

	droplet, err := createDroplet(c.UI, c.region, c.size, publicKey, name, environment)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error creating server: %s", err))
		return 1
	}

	c.UI.Info("✓ Server booted")

	droplet, err = waitForSSH(droplet)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Timeout waiting for SSH: %s", err))
		return 1
	}

	c.UI.Info("✓ SSH available")

	ip, _ := droplet.PublicIPv4()

	_, err = c.Trellis.UpdateHosts(environment, ip)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error updating Trellis hosts file: %s", err))
		return 1
	}

	c.UI.Info(fmt.Sprintf("✓ Updated hosts/%s with IP: %s", environment, ip))

	if c.skipProvision {
		c.UI.Warn(fmt.Sprintf("Skipping provision. Run `trellis provision %s` to manually provision.", environment))
	} else {
		c.UI.Info("\nInstalling Galaxy roles...\n")
		galaxyInstallCmd := GalaxyInstallCommand{UI: c.UI, Trellis: c.Trellis}
		galaxyInstallCmd.Run([]string{})

		c.UI.Info("\nProvisioning server...\n")

		provisionCmd := NewProvisionCommand(c.UI, c.Trellis)
		return provisionCmd.Run([]string{environment})
	}

	return 0
}

func askDropletName(ui cli.Ui, siteName string) (name string, err error) {
	name, err = ui.Ask(fmt.Sprintf("Droplet name [%s]:", color.GreenString(siteName)))
	if err != nil {
		return "", err
	}

	if name == "" {
		name = siteName
	}

	return name, nil
}

func getAccessToken(ui cli.Ui) (accessToken string, err error) {
	accessToken = os.Getenv("DIGITALOCEAN_ACCESS_TOKEN")

	if accessToken == "" {
		ui.Info("DIGITALOCEAN_ACCESS_TOKEN environment variable not set.")
		accessToken, err = ui.Ask("Enter Access token:")

		if err != nil {
			return "", err
		}

		_ = os.Setenv("DIGITALOCEAN_ACCESS_TOKEN", accessToken)
	}

	return accessToken, nil
}

func createDroplet(ui cli.Ui, region string, size string, publicKey ssh.PublicKey, name string, env string) (droplet *godo.Droplet, err error) {
	droplet, monitorUri, err := client.CreateDroplet(region, size, publicKey, name, env)
	if err != nil {
		return nil, err
	}

	ui.Info(fmt.Sprintf("\n✓ Server created => https://cloud.digitalocean.com/droplets/%d", droplet.ID))

	s := spinner.New(spinner.CharSets[33], 100*time.Millisecond)
	s.Suffix = " Waiting for server to boot (this may take a minute)"

	s.Start()
	err = util.WaitForActive(context.TODO(), client.Client, monitorUri)
	s.Stop()

	if err != nil {
		return nil, err
	}

	return droplet, nil
}

func waitForSSH(droplet *godo.Droplet) (*godo.Droplet, error) {
	const retries int = 9 // retry for a total of 3 minutes

	droplet, ip, err := client.GetDroplet(droplet)
	if err != nil {
		return droplet, err
	}

	s := spinner.New(spinner.CharSets[33], 100*time.Millisecond)
	s.Suffix = " Waiting for SSH (this may take a minute)"
	s.Start()

	for i := 0; i < retries; i++ {
		if checkSSH(ip) == nil {
			break
		}

		time.Sleep(10 * time.Second)
	}

	s.Stop()

	return droplet, nil
}

func checkSSH(host string) (err error) {
	host = net.JoinHostPort(host, "22")
	_, err = net.DialTimeout("tcp", host, 10*time.Second)
	return err
}

func checkSSHKey(ui cli.Ui, keyString string, publicKey ssh.PublicKey) error {
	response, err := client.GetSSHKey(publicKey)

	switch response.StatusCode {
	case 404:
		ui.Info("SSH Key does not exist in DigitalOcean.")

		prompt := promptui.Prompt{
			Label:     "Add SSH key to account",
			IsConfirm: true,
		}

		_, err := prompt.Run()

		if err != nil {
			return errors.New("Can't continue without an SSH key on your account.")
		}

		return client.CreateSSHKey(keyString)
	case 200:
		return nil
	default:
		return err
	}

	return nil
}

func loadSSHKey(path string) (keyString string, publicKey ssh.PublicKey, err error) {
	path, err = homedir.Expand(path)
	key, err := ioutil.ReadFile(path)
	if err != nil {
		return "", nil, err
	}

	publicKey, _, _, _, err = ssh.ParseAuthorizedKey(key)
	if err != nil {
		return "", nil, err
	}

	keyString = string(key)

	return keyString, publicKey, nil
}

func selectRegion() (region *godo.Region, err error) {
	availableRegions, err := client.GetAvailableRegions()
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

func selectSize(region *godo.Region) (size string, err error) {
	sizes, err := client.GetSizesByRegion(region)
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

Create a production server:

  $ trellis droplet create production

Create a 1gb server in the nyc3 region:

  $ trellis droplet create --region=nyc3 --size=s-1vcpu-1gb production

Create a server but skip provisioning:

  $ trellis droplet create --skip-provision production

Arguments:
  ENVIRONMENT Name of environment (ie: production)

Options:
      --region          Region to create the server in
      --size            Server size/type
      --skip-provision  Skip provision after server is created
      --ssh-key         (default: ~/.ssh/id_rsa.pub) path to SSH public key to be added on the server
  -h, --help            show this help
`

	return strings.TrimSpace(helpText)
}

func (c *DropletCreateCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.PredictEnvironment()
}

func (c *DropletCreateCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--region":          complete.PredictNothing,
		"--size":            complete.PredictNothing,
		"--skip--provision": complete.PredictNothing,
		"--ssh-key":         complete.PredictFiles("*"),
	}
}
