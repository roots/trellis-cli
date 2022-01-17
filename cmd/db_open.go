package cmd

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/command"
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

	c := &DBOpenCommand{UI: ui, Trellis: trellis, dbOpenerFactory: &DBOpenerFactory{}, playbook: playbook}
	c.init()
	return c
}

type DBOpenCommand struct {
	UI              cli.Ui
	flags           *flag.FlagSet
	app             string
	Trellis         *trellis.Trellis
	dbOpenerFactory *DBOpenerFactory
	playbook        *AdHocPlaybook
}

type DBCredentials struct {
	SSHUser    string `json:"web_user"`
	SSHHost    string `json:"ansible_host"`
	SSHPort    int    `json:"ansible_port"`
	DBUser     string `json:"db_user"`
	DBPassword string `json:"db_password"`
	DBHost     string `json:"db_host"`
	DBName     string `json:"db_name"`
	WPEnv      string `json:"wp_env"`
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

	commandArgumentValidator := &CommandArgumentValidator{required: 1, optional: 1}
	if err := commandArgumentValidator.validate(args); err != nil {
		c.UI.Error(err.Error())
		c.UI.Output(c.Help())
		return 1
	}

	environment := c.flags.Arg(0)
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

	// Initialize db opener object; check app is supported.
	opener, dbOpenerFactoryMakeErr := c.dbOpenerFactory.Make(c.app, c.UI)
	if dbOpenerFactoryMakeErr != nil {
		c.UI.Error(fmt.Sprintf("Error initializing new db opener object: %s", dbOpenerFactoryMakeErr))
		c.UI.Error(fmt.Sprintf("Supported apps are: %s", c.dbOpenerFactory.GetSupportedApps()))
		return 1
	}

	// Prepare JSON file for db credentials
	dbCredentialsJson, dbCredentialsErr := ioutil.TempFile("", "*.json")
	if dbCredentialsErr != nil {
		c.UI.Error(fmt.Sprintf("Error creating temporary db credentials JSON file: %s", dbCredentialsErr))
	}
	defer os.Remove(dbCredentialsJson.Name())

	defer c.playbook.DumpFiles()()

	// Template db credentials to JSON file.
	playbookArgs := []string{
		"dump_db_credentials.yml",
		"-e", "env=" + environment,
		"-e", "site=" + siteName,
		"-e", "dest=" + dbCredentialsJson.Name(),
	}
	dumpDbCredentials := command.Cmd("ansible-playbook", playbookArgs)

	if err := dumpDbCredentials.Run(); err != nil {
		c.UI.Error(fmt.Sprintf("Error running ansible-playbook dump_db_credentials.yml: %s", err))
		return 1
	}

	// Read db credentials from JSON file.
	dbCredentialsByte, readErr := ioutil.ReadFile(dbCredentialsJson.Name())
	if readErr != nil {
		c.UI.Error(fmt.Sprintf("Error reading db credentials JSON file: %s", readErr))
		return 1
	}
	var dbCredentials DBCredentials
	unmarshalErr := json.Unmarshal(dbCredentialsByte, &dbCredentials)
	if unmarshalErr != nil {
		c.UI.Error(fmt.Sprintf("Error unmarshaling db credentials JSON file: %s", unmarshalErr))
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
Usage: trellis db [options] ENVIRONMENT [SITE]

Open database with GUI applications

Open default site's production database with tableplus:

  $ trellis db open --app=tableplus production

Open a site's production database with tableplus:

  $ trellis db open --app=tableplus production example.com

Arguments:
  ENVIRONMENT Name of environment (ie: production)
  SITE        Name of the site (ie: example.com); Optional when only single site exist in the environment

Options:
      --app         Database client to be open with; Supported: %s
  -h, --help        show this help
`

	return strings.TrimSpace(fmt.Sprintf(helpText, c.dbOpenerFactory.GetSupportedApps()))
}
