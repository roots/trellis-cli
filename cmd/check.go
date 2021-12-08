package cmd

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

type CheckCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

var Requirements = []trellis.Requirement{
	{
		Name:              "Python",
		Command:           "python",
		Url:               "https://www.python.org/",
		VersionConstraint: ">= 2.7.0",
		ExtractVersion: func(output string) string {
			return strings.Replace(output, "Python ", "", 1)
		},
	},
	{
		Name:              "Vagrant",
		Command:           "vagrant",
		Url:               "https://www.vagrantup.com/downloads.html",
		VersionConstraint: ">= 2.1.0",
		ExtractVersion: func(output string) string {
			return strings.Replace(output, "Vagrant ", "", 1)
		},
	},
	{
		Name:              "VirtualBox",
		Command:           "VBoxManage",
		Url:               "https://www.virtualbox.org/wiki/Downloads",
		VersionConstraint: ">= 4.3.10",
	},
}

func (c *CheckCommand) Run(args []string) int {
	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	c.UI.Info("Checking Trellis requirements\n")

	requirementsMet := 0

	for _, req := range Requirements {
		output := fmt.Sprintf("%s [%s]:", req.Name, req.VersionConstraint)

		result, err := req.Check()
		if err != nil {
			c.UI.Error(fmt.Sprintf("Error checking %s requirement: %s", req.Name, err))
		}

		if result.Installed {
			if result.Satisfied {
				requirementsMet += 1
				output = fmt.Sprintf("%s %s %s", color.GreenString("[✓]"), output, color.GreenString(result.Version))
			} else {
				output = fmt.Sprintf("%s %s %s", color.RedString("[X]"), output, color.RedString(result.Version))
			}
		} else {
			output = fmt.Sprintf("%s %s", output, color.RedString("not installed"))
		}

		c.UI.Info(output)
	}

	if requirementsMet == len(Requirements) {
		c.UI.Info("\nAll requirements met")
		return 0
	} else {
		c.UI.Error(fmt.Sprintf("\n%d requirement(s) not met\n", len(Requirements)-requirementsMet))
		c.UI.Info("See https://roots.io/trellis/docs/installing-trellis/#install-requirements")
		return 1
	}
}

func (c *CheckCommand) Synopsis() string {
	return "Checks if Trellis requirements are met"
}

func (c *CheckCommand) Help() string {
	helpText := `
Usage: trellis check

Checks if Trellis requirements are met

Options:
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}
