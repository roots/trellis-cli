package cmd

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

type DotEnvCommand struct {
	UI       cli.Ui
	Trellis  *trellis.Trellis
	playbook *AdHocPlaybook
}

//go:embed files/playbooks/dot_env_template.yml
var dotenvYmlContent string

func NewDotEnvCommand(ui cli.Ui, trellis *trellis.Trellis) *DotEnvCommand {
	playbook := &AdHocPlaybook{
		path: trellis.Path,
		files: map[string]string{
			"dotenv.yml": dotenvYmlContent,
		},
	}

	return &DotEnvCommand{UI: ui, Trellis: trellis, playbook: playbook}
}

func (c *DotEnvCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 1}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	spinner := NewSpinner(
		SpinnerCfg{
			Message:     "Validating ansible-vault usability",
			StopMessage: "ansible-vault is ready to use",
			FailMessage: "ansible-vault is not ready to use",
		},
	)

	spinner.Start()

	if err := validateAnsibleVaultUsable(c.Trellis.Path); err != nil {
		spinner.StopFail()
		c.UI.Error(fmt.Sprintf("Error validating ansible-vault usability: %s", err))
		return 1
	}

	spinner.Stop()

	spinner = NewSpinner(
		SpinnerCfg{
			Message:     "Generating .env file",
			StopMessage: "Generated .env file",
			FailMessage: "Error templating .env file",
		},
	)
	spinner.Start()

	environment := "development"
	if len(args) == 1 {
		environment = args[0]
	}

	environmentErr := c.Trellis.ValidateEnvironment(environment)
	if environmentErr != nil {
		c.UI.Error(environmentErr.Error())
		return 1
	}

	defer c.playbook.DumpFiles()()

	dotenv := command.Cmd("ansible-playbook", []string{"dotenv.yml", "-e", "env=" + environment})

	if err := dotenv.Run(); err != nil {
		spinner.StopFail()
		c.UI.Error(fmt.Sprintf("Error running ansible-playbook: %s", err))
		return 1
	}

	spinner.Stop()
	return 0
}

func (c *DotEnvCommand) Synopsis() string {
	return "Template .env files to local system"
}

func (c *DotEnvCommand) Help() string {
	helpText := `
Usage: trellis dotenv [options] [ENVIRONMENT=development]

Template .env files to local system

Options:
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}
