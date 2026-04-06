package cmd

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/hashicorp/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/ansible"
	"github.com/roots/trellis-cli/pkg/db_opener"
	"github.com/roots/trellis-cli/trellis"
)

//go:embed files/playbooks/db_credentials.yml
var dumpDbCredentialsYml string

//go:embed files/db_credentials_template.yml
var dbCredentialsJsonJ2 string

func NewDBOpenCommand(ui cli.Ui, trellis *trellis.Trellis) *DBOpenCommand {
	playbook := &AdHocPlaybook{
		path: trellis.Path,
		files: map[string]string{
			"dump_db_credentials.yml": dumpDbCredentialsYml,
			"db_credentials.json.j2":  dbCredentialsJsonJ2,
		},
	}

	c := &DBOpenCommand{UI: ui, Trellis: trellis, dbOpenerFactory: &db_opener.Factory{}, playbook: playbook}
	c.init()
	return c
}

type DBOpenCommand struct {
	UI              cli.Ui
	flags           *flag.FlagSet
	app             string
	Trellis         *trellis.Trellis
	dbOpenerFactory *db_opener.Factory
	playbook        *AdHocPlaybook
}

func (c *DBOpenCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	appUsage := fmt.Sprintf("Database client to be used; Supported: %s", c.dbOpenerFactory.GetSupportedApps())
	c.flags.StringVar(&c.app, "app", "", appUsage)
}

func (c *DBOpenCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 2}
	if err := commandArgumentValidator.validate(args); err != nil {
		c.UI.Error(err.Error())
		c.UI.Output(c.Help())
		return 1
	}

	environment := c.flags.Arg(0)

	if environment == "" {
		environment = "development"
	}

	if err := c.Trellis.ValidateEnvironment(environment); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	siteNameArg := c.flags.Arg(1)
	siteName, siteNameErr := c.Trellis.FindSiteNameFromEnvironment(environment, siteNameArg)
	if siteNameErr != nil {
		c.UI.Error(siteNameErr.Error())
		return 1
	}

	if c.Trellis.CliConfig.DatabaseApp != "" && c.app == "" {
		c.app = c.Trellis.CliConfig.DatabaseApp
	}

	// Initialize db opener object; check app is supported.
	opener, dbOpenerFactoryMakeErr := c.dbOpenerFactory.Make(c.app)
	if dbOpenerFactoryMakeErr != nil {
		c.UI.Error(fmt.Sprintf("Error initializing new db opener object: %s", dbOpenerFactoryMakeErr))
		c.UI.Error(fmt.Sprintf("Supported apps are: %s", c.dbOpenerFactory.GetSupportedApps()))
		return 1
	}

	var dbCredentialsByte []byte
	var mockUi *cli.MockUi

	// For WSL development, run ansible-playbook inside the distro since
	// Ansible is not installed on the Windows host.
	if environment == "development" && runtime.GOOS == "windows" && c.Trellis.VmManagerType() == "wsl" {
		instanceName, err := c.Trellis.GetVmInstanceName()
		if err != nil {
			c.UI.Error(err.Error())
			return 1
		}
		distro := "trellis-" + strings.ReplaceAll(instanceName, ".", "-")

		// c.Trellis.Path is the trellis dir (e.g. C:\...\testsite.com\trellis).
		// Project root is the parent directory.
		projectRoot := filepath.Dir(c.Trellis.Path)
		projectName := filepath.Base(projectRoot)

		wslProjectRoot := "/home/admin/" + projectName
		wslTrellisDir := wslProjectRoot + "/trellis"
		wslDest := "/tmp/trellis-db-credentials.json"

		// Dump the ad-hoc playbook files into the Windows trellis dir
		// so they're accessible from WSL via the synced ext4 copy.
		defer c.playbook.DumpFiles()()

		// Sync the playbook files to WSL so the distro has them.
		syncScript := fmt.Sprintf(
			`cp %s/dump_db_credentials.yml %s/dump_db_credentials.yml && cp %s/db_credentials.json.j2 %s/db_credentials.json.j2`,
			toWslPath(c.Trellis.Path), wslTrellisDir,
			toWslPath(c.Trellis.Path), wslTrellisDir,
		)
		_ = command.Cmd("wsl", []string{
			"-d", distro, "-u", "admin",
			"--", "bash", "-c", syncScript,
		}).Run()

		// Build the ansible-playbook command to run inside WSL.
		playbookCmd := fmt.Sprintf(
			"cd %s && ansible-playbook dump_db_credentials.yml -e env=%s -e site=%s -e dest=%s --inventory=%s/inventory",
			wslTrellisDir, environment, siteName, wslDest, wslTrellisDir+"/.trellis/wsl",
		)

		mockUi = cli.NewMockUi()
		dumpDbCredentials := command.WithOptions(
			command.WithUiOutput(mockUi),
		).Cmd("wsl", []string{
			"-d", distro, "-u", "admin",
			"--", "bash", "-c", playbookCmd,
		})

		if err := dumpDbCredentials.Run(); err != nil {
			c.UI.Error("Error opening database. Temporary playbook failed to execute:")
			c.UI.Error(mockUi.OutputWriter.String())
			c.UI.Error(mockUi.ErrorWriter.String())
			return 1
		}

		// Read the JSON result file from inside the distro.
		output, err := command.Cmd("wsl", []string{
			"-d", distro, "-u", "admin",
			"--", "cat", wslDest,
		}).Output()
		if err != nil {
			c.UI.Error("Error reading db credentials from WSL distro.")
			return 1
		}
		dbCredentialsByte = output

		// Clean up the temp file inside WSL.
		_ = command.Cmd("wsl", []string{
			"-d", distro, "-u", "admin",
			"--", "rm", "-f", wslDest,
		}).Run()

		// Clean up the ad-hoc playbook files inside WSL.
		_ = command.Cmd("wsl", []string{
			"-d", distro, "-u", "admin",
			"--", "rm", "-f",
			wslTrellisDir + "/dump_db_credentials.yml",
			wslTrellisDir + "/db_credentials.json.j2",
		}).Run()
	} else {
		// Standard path: run ansible-playbook on the host.
		dbCredentialsJson, dbCredentialsErr := os.CreateTemp("", "*.json")
		if dbCredentialsErr != nil {
			c.UI.Error(fmt.Sprintf("Error creating temporary db credentials JSON file: %s", dbCredentialsErr))
			return 1
		}
		defer os.Remove(dbCredentialsJson.Name())

		defer c.playbook.DumpFiles()()

		playbook := ansible.Playbook{
			Name: "dump_db_credentials.yml",
			Env:  environment,
			ExtraVars: map[string]string{
				"site": siteName,
				"dest": dbCredentialsJson.Name(),
			},
		}

		if environment == "development" {
			playbook.SetInventory(c.Trellis.VmInventoryPath())
		}

		mockUi = cli.NewMockUi()
		dumpDbCredentials := command.WithOptions(
			command.WithUiOutput(mockUi),
		).Cmd("ansible-playbook", playbook.CmdArgs())

		if err := dumpDbCredentials.Run(); err != nil {
			c.UI.Error("Error opening database. Temporary playbook failed to execute:")
			c.UI.Error(mockUi.OutputWriter.String())
			c.UI.Error(mockUi.ErrorWriter.String())
			return 1
		}

		var readErr error
		dbCredentialsByte, readErr = os.ReadFile(dbCredentialsJson.Name())
		if readErr != nil {
			c.UI.Error(fmt.Sprintf("Error reading db credentials JSON file: %s", readErr))
			return 1
		}
	}

	var dbCredentials db_opener.DBCredentials
	unmarshalErr := json.Unmarshal(dbCredentialsByte, &dbCredentials)
	if unmarshalErr != nil {
		c.UI.Error(fmt.Sprintf("Error parsing db credentials JSON file: %s", unmarshalErr))
		c.UI.Error("This probably means the temporary playbook used to template out the JSON file with database credentials failed. Here was the output for troubleshooting:")
		c.UI.Error(mockUi.OutputWriter.String())
		c.UI.Error(mockUi.ErrorWriter.String())
		return 1
	}

	// Open database with GUI application.
	if err := opener.Open(dbCredentials); err != nil {
		c.UI.Error(fmt.Sprintf("Error opening db: %s", err))
		return 1
	}

	c.UI.Info(color.GreenString(fmt.Sprintf("[✓] Open %s (%s) database with %s", siteName, environment, c.app)))
	return 0
}

func (c *DBOpenCommand) Synopsis() string {
	return "Open database with GUI applications"
}

func (c *DBOpenCommand) Help() string {
	helpText := `
Usage: trellis db [options] [ENVIRONMENT=development] [SITE]

Open database with GUI applications (defaults to development environment).

Open default site's production database with tableplus:

  $ trellis db open --app tableplus production

Open a site's production database with Sequel Ace:

  $ trellis db open --app sequel-ace production example.com

To set a default database app, set the 'databae_app' option in your CLI (project or global) config file:

  database_app: sequel-ace

Arguments:
  ENVIRONMENT Name of environment (default: development)
  SITE        Name of the site (ie: example.com); Optional when only single site exist in the environment

Options:
      --app         Database client to be open with; Supported: %s
  -h, --help        show this help
`

	return strings.TrimSpace(fmt.Sprintf(helpText, c.dbOpenerFactory.GetSupportedApps()))
}

func (c *DBOpenCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteSite(c.flags)
}

func (c *DBOpenCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--app": complete.PredictSet(c.dbOpenerFactory.GetSupportedApps()...),
	}
}

// toWslPath converts a Windows path to a WSL mount path.
func toWslPath(windowsPath string) string {
	p := filepath.ToSlash(windowsPath)
	if len(p) >= 2 && p[1] == ':' {
		p = "/mnt/" + strings.ToLower(string(p[0])) + p[2:]
	}
	return p
}
