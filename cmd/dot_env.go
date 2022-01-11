package cmd

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

type DotEnvCommand struct {
	UI       cli.Ui
	Trellis  *trellis.Trellis
	playbook PlaybookRunner
}

//go:embed files/playbooks/dot_env_template.yml
var dotenvYmlContent string

func NewDotEnvCommand(ui cli.Ui, trellis *trellis.Trellis) *DotEnvCommand {
	playbook := &AdHocPlaybook{
		files: map[string]string{
			"dotenv.yml": dotenvYmlContent,
		},
		Playbook: Playbook{
			ui: cli.NewMockUi(),
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

	environment := "development"
	if len(args) == 1 {
		environment = args[0]
	}

	environmentErr := c.Trellis.ValidateEnvironment(environment)
	if environmentErr != nil {
		c.UI.Error(environmentErr.Error())
		return 1
	}

	spinner := NewSpinner(
		SpinnerCfg{
			Message:     "Generating .env file",
			StopMessage: "Generated .env file",
			FailMessage: "Error templating .env file",
		},
	)
	spinner.Start()

	c.playbook.SetRoot(c.Trellis.Path)

	if err := c.playbook.Run("dotenv.yml", []string{"-e", "env=" + environment}); err != nil {
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
