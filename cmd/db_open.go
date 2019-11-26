package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/mitchellh/cli"
	"trellis-cli/templates"
	"trellis-cli/trellis"
)

const playbookPath = "dump_db_credentials.yml"
const dumpDbCredentialsYmlTemplate = `
---
- name: 'Trellis CLI: Dump database credentials'
  hosts: web:&{{ env }}
  remote_user: "{{ web_user }}"
  gather_facts: false
  connection: local
  tasks:
    - name: Dump database credentials
      template:
        src: db_credentials.json.j2
        dest: "{{ dest }}"
        mode: '0600'
      with_dict: "{{ wordpress_sites }}"
      when: item.key == site
`

const j2TemplatePath = "db_credentials.json.j2"
const dbCredentialsJsonJ2Template = `
{
    "web_user": "{{ web_user }}",
    "ansible_host": "{{ ansible_host }}",
    "ansible_port": {{ ansible_port | default(22) }},
    "db_user": "{{ site_env.db_user }}",
    "db_password": "{{ site_env.db_password }}",
    "db_host": "{{ site_env.db_host }}",
    "db_name": "{{ site_env.db_name }}",
    "wp_env": "{{ site_env.wp_env }}"
}
`

func NewDBOpenCommand(ui cli.Ui, trellis *trellis.Trellis, dbOpenerFactory *DBOpenerFactory) *DBOpenCommand {
	c := &DBOpenCommand{UI: ui, Trellis: trellis, dbOpenerFactory: dbOpenerFactory}
	c.init()
	return c
}

type DBOpenCommand struct {
	UI              cli.Ui
	flags           *flag.FlagSet
	app             string
	Trellis         *trellis.Trellis
	dbOpenerFactory *DBOpenerFactory
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
	c.flags.StringVar(&c.app, "app", "", "Database client to be used; Supported: tableplus, sequel-pro")
}

func (c *DBOpenCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if flagsParseErr := c.flags.Parse(args); flagsParseErr != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 1, optional: 1}
	if commandArgumentErr := commandArgumentValidator.validate(args); commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	environment := c.flags.Arg(0)
	environmentErr := c.Trellis.ValidateEnvironment(environment)
	if environmentErr != nil {
		c.UI.Error(environmentErr.Error())
		return 1
	}

	siteNameArg := c.flags.Arg(1)
	siteName, siteNameErr := c.Trellis.FindSiteNameFromEnvironment(environment, siteNameArg)
	if siteNameErr != nil {
		c.UI.Error(siteNameErr.Error())
		return 1
	}

	// Template JSON file for db credentials
	dbCredentialsJson, dbCredentialsErr := ioutil.TempFile("", "*.json")
	if dbCredentialsErr != nil {
		c.UI.Error(fmt.Sprintf("Error createing temporary db credentials JSON file: %s", dbCredentialsErr))
	}
	defer deleteFile(dbCredentialsJson.Name())

	// Template playbook files from package to Trellis
	writeFile(playbookPath, templates.TrimSpace(dumpDbCredentialsYmlTemplate))
	defer deleteFile(playbookPath)
	writeFile(j2TemplatePath, templates.TrimSpace(dbCredentialsJsonJ2Template))
	defer deleteFile(j2TemplatePath)

	// Run the playbook to generate dbCredentialsJson
	playbookCommand := execCommand("ansible-playbook", playbookPath, "-e", "env="+environment, "-e", "site="+siteName, "-e", "dest="+dbCredentialsJson.Name())
	appendEnvironmentVariable(playbookCommand, "ANSIBLE_RETRY_FILES_ENABLED=false")
	logCmd(playbookCommand, c.UI, true)
	playbookErr := playbookCommand.Run()
	if playbookErr != nil {
		c.UI.Error(fmt.Sprintf("Error running ansible-playbook: %s", playbookErr))
		return 1
	}

	// Read dbCredentialsJson
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

	// Open database with dbCredentialsJson and the app
	opener, newDBOpenerErr := c.dbOpenerFactory.make(c.app, c.UI)
	if newDBOpenerErr != nil {
		c.UI.Error(fmt.Sprintf("Error initializing new db opener object: %s", newDBOpenerErr))
		return 1
	}

	openErr := opener.open(dbCredentials)
	if openErr != nil {
		c.UI.Error(fmt.Sprintf("Error opening db: %s", openErr))
		return 1
	}

	return 0
}

func (c *DBOpenCommand) Synopsis() string {
	return "Open database with GUI applications"
}

func (c *DBOpenCommand) Help() string {
	helpText := `
Usage: trellis db [options] ENVIRONMENT [SITE]

Open database with GUI applications

Open a site's production database with tableplus:

  $ trellis db open --app=tableplus production example.com

Arguments:
  ENVIRONMENT Name of environment (ie: production)
  SITE        Name of the site (ie: example.com); Optional when only single site exist in the environment

Options:
      --app         Database client to be open with; Supported: tableplus, sequel-pro
  -h, --help        show this help
`

	return strings.TrimSpace(helpText)
}
