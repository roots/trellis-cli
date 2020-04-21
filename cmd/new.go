package cmd

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/mitchellh/cli"
	"github.com/weppos/publicsuffix-go/publicsuffix"
	"trellis-cli/github"
	"trellis-cli/trellis"
)

type NewCommand struct {
	UI             cli.Ui
	CliVersion     string
	flags          *flag.FlagSet
	trellis        *trellis.Trellis
	force          bool
	name           string
	host           string
	skipVirtualenv bool
	vaultPass      string
}

func NewNewCommand(ui cli.Ui, trellis *trellis.Trellis, version string) *NewCommand {
	c := &NewCommand{UI: ui, trellis: trellis, CliVersion: version}
	c.init()
	return c
}

func (c *NewCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.force, "force", false, "Forces the creation of the project even if the target path is not empty")
	c.flags.StringVar(&c.name, "name", "", "Main site name (the domain name). Bypasses the name prompt if specified. Example: mydomain.com")
	c.flags.StringVar(&c.host, "host", "", "Main site hostname. Bypasses the host prompt if specified. Example: mydomain.com or www.mydomain.com")
	c.flags.StringVar(&c.vaultPass, "vault-pass", ".vault_pass", "Path for the generated Vault pass file")
	c.flags.BoolVar(&c.skipVirtualenv, "skip-virtualenv", false, "Skip creating a new virtual environment for this project")
}

func (c *NewCommand) Run(args []string) int {
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

	path := args[0]

	path, _ = filepath.Abs(path)
	fi, statErr := os.Stat(path)

	c.UI.Info(fmt.Sprintf("Creating new Trellis project in %s\n", path))

	if !c.force && statErr == nil && fi.IsDir() {
		isPathEmpty, _ := isDirEmpty(path)

		if !isPathEmpty {
			c.UI.Error(fmt.Sprintf("%s path is not empty. Use --force option to skip this check.", path))
			return 1
		}
	}

	if c.name == "" {
		name, err := askDomain(c.UI, path)
		if err != nil {
			c.UI.Error(fmt.Sprintf("Error: %s", err.Error()))
			return 1
		}

		c.name = name
	}

	if c.host == "" {
		host, err := askHost(c.UI, c.trellis, c.name)
		if err != nil {
			return 1
		}

		c.host = host
	}

	if statErr != nil {
		if os.IsNotExist(statErr) {
			if err := os.MkdirAll(path, os.ModePerm); err != nil {
				c.UI.Error(fmt.Sprintf("Error creating directory: %s", err))
				return 1
			}
		} else {
			c.UI.Error(fmt.Sprintf("Error reading path %s", statErr))
			return 1
		}
	}

	fmt.Println("\nFetching latest versions of Trellis and Bedrock...")

	trellisPath := filepath.Join(path, "trellis")
	trellisVersion := github.DownloadLatestRelease("roots/trellis", path, trellisPath)
	bedrockVersion := github.DownloadLatestRelease("roots/bedrock", path, filepath.Join(path, "site"))

	os.Chdir(path)

	if err := c.trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if !c.skipVirtualenv {
		initCommand := &InitCommand{UI: c.UI, Trellis: c.trellis}
		initCommand.Run([]string{})
	}

	// Update default configs
	for env, config := range c.trellis.Environments {
		c.trellis.UpdateDefaultConfig(config, c.name, c.host, env)
		c.trellis.WriteYamlFile(
			config,
			filepath.Join("group_vars", env, "wordpress_sites.yml"),
			c.YamlHeader("https://roots.io/trellis/docs/wordpress-sites/"),
		)

		stringGenerator := trellis.RandomStringGenerator{Length: 64}
		vault := c.trellis.GenerateVaultConfig(c.name, env, &stringGenerator)
		c.trellis.WriteYamlFile(
			vault,
			filepath.Join("group_vars", env, "vault.yml"),
			c.YamlHeader("https://roots.io/trellis/docs/vault/"),
		)
	}

	if err := c.trellis.GenerateVaultPassFile(c.vaultPass); err != nil {
		c.UI.Error(fmt.Sprintf("Error writing Vault pass file: %s", err))
	}

	if err := c.trellis.UpdateAnsibleConfig("defaults", "vault_password_file", c.vaultPass); err != nil {
		c.UI.Error(fmt.Sprintf("Error adding vault_password_file setting to ansible.cfg: %s", err))
	}

	galaxyInstallCommand := &GalaxyInstallCommand{c.UI, c.trellis}
	galaxyInstallCommand.Run([]string{})

	fmt.Printf("\n%s project created with versions:\n", color.GreenString(c.name))
	fmt.Printf("  Trellis %s\n", trellisVersion)
	fmt.Printf("  Bedrock v%s\n", bedrockVersion)

	return 0
}

func (c *NewCommand) Synopsis() string {
	return "Creates a new Trellis project"
}

func (c *NewCommand) Help() string {
	helpText := `
Usage: trellis new [options] [PATH]

Creates a new Trellis project in the path specified using the latest versions of Trellis and Bedrock.

This uses our recommended project structure detailed at
https://roots.io/trellis/docs/installing-trellis/#create-a-project

Create a new project in the current directory:

  $ trellis new .

Create a new project in the target path:

  $ trellis new ~/dev/example.com

Force create a new project in a non-empty target path:

  $ trellis new --force ~/dev/example.com

Specify name and host to bypass the prompts:

  $ trellis new --name example.com --host www.example.com ~/dev/foo

Arguments:
  PATH  Path to create new project in

Options:
      --force            (default: false) Forces the creation of the project even if the target path is not empty
      --name             Main site name (the domain name). Bypasses the name prompt if specified. Example: mydomain.com
      --host             Main site hostname. Bypasses the host prompt if specified. Example: mydomain.com or www.mydomain.com
      --skip-virtualenv  (default: false) Skip creating a new virtual environment for this project
      --vault-pass       (default: .vault_pass) Path for the generated Vault pass file
  -h, --help             show this help
`

	return strings.TrimSpace(helpText)
}

func (c *NewCommand) YamlHeader(doc string) string {
	const header = "# Created by trellis-cli v%s\n# Documentation: %s\n\n"

	return fmt.Sprintf(header, c.CliVersion, doc)
}

func addTrellisFile(path string) error {
	path = filepath.Join(path, ".trellis.yml")
	return ioutil.WriteFile(path, []byte{}, 0666)
}

func askDomain(ui cli.Ui, path string) (host string, err error) {
	path = filepath.Base(path)
	domain, err := publicsuffix.Parse(path)

	if err != nil {
		return "", fmt.Errorf("path `%s` must be a valid domain name (ie: `example.com` and not just `example`)", path)
	}

	if domain.TRD == "www" {
		domain.TRD = ""
	}

	domainName := domain.String()

	host, err = ui.Ask(fmt.Sprintf("%s [%s]:", color.MagentaString("Site domain"), color.GreenString(domainName)))

	if err != nil {
		return "", err
	}

	if len(host) == 0 {
		return domainName, nil
	}

	return host, nil
}

func askHost(ui cli.Ui, t *trellis.Trellis, name string) (host string, err error) {
	domain, wwwDomain := t.HostsFromDomain(name, "production")
	items := []string{domain.String()}
	index := -1
	var result string

	if wwwDomain != nil {
		items = append(items, wwwDomain.String())
	}

	fmt.Println("")

	for index < 0 {
		prompt := promptui.SelectWithAdd{
			Label:    "Select main site host",
			Items:    items,
			AddLabel: "Other",
			HideHelp: true,
		}
		index, result, err = prompt.Run()

		if err != nil {
			return "", err
		}

		if index == -1 {
			items = append(items, result)
		}
	}

	return result, nil
}

func isDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)

	if err != nil {
		return false, err
	}

	defer f.Close()

	if _, err = f.Readdirnames(1); err == io.EOF {
		return true, nil
	}

	return false, err
}
