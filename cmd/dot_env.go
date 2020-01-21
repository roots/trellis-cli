package cmd

import (
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

type DotEnvCommand struct {
	UI       cli.Ui
	Trellis  *trellis.Trellis
	playbook PlaybookRunner
}

const dotenvYmlContent = `
---
- name: 'Trellis CLI: Template .env files to local system'
  hosts: web:&{{ env }}
  connection: local
  gather_facts: false
  tasks:
    - name: Template .env files to local system
      template:
        src: roles/deploy/templates/env.j2
        dest: "{{ item.value.local_path }}/.env"
        mode: '0644'
      with_dict: "{{ wordpress_sites }}"
`

func NewDotEnvCommand(ui cli.Ui, trellis *trellis.Trellis) *DotEnvCommand {
	playbook := &AdHocPlaybook{
		files: map[string]string{
			"dotenv.yml": dotenvYmlContent,
		},
		Playbook: Playbook{
			ui: ui,
		},
	}

	return &DotEnvCommand{UI: ui, Trellis: trellis, playbook: playbook}
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

	environmentErr := c.Trellis.ValidateEnvironment(environment)
	if environmentErr != nil {
		c.UI.Error(environmentErr.Error())
		return 1
	}

	c.playbook.SetRoot(c.Trellis.Path)

	if err := c.playbook.Run("dotenv.yml", []string{"-e", "env=" + environment}); err != nil {
		c.UI.Error(fmt.Sprintf("Error running ansible-playbook: %s", err))
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
