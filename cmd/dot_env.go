package cmd

import (
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	. "trellis-cli/templates"
	"trellis-cli/trellis"
)

type DotEnvCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *DotEnvCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

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

	_, ok := c.Trellis.Environments[environment]
	if !ok {
		c.UI.Error(fmt.Sprintf("Error: %s is not a valid environment", environment))
		return 1
	}

	// Template playbook file from package to Trellis
	playbookPath := "dotenv.yml"
	writeFile(playbookPath, TrimSpace(DOTENV_YML))
	defer deleteFile(playbookPath)

	dotEnv := execCommand("ansible-playbook", "dotenv.yml", "-e", "env=" + environment)
	appendEnvironmentVariable(dotEnv, "ANSIBLE_RETRY_FILES_ENABLED=false")

	logCmd(dotEnv, c.UI, true)
	runErr := dotEnv.Run()

	if runErr != nil {
		c.UI.Error(fmt.Sprintf("Error running ansible-playbook: %s", runErr))
		return 1
	}

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
