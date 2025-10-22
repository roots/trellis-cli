package cmd

import (
	"fmt"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/trellis"
)

type CheckCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

var Requirements = []trellis.Requirement{
	{
		Name:              "Python",
		Command:           "python3",
		Optional:          false,
		Url:               "https://www.python.org/",
		VersionConstraint: ">= 3.8.0",
		ExtractVersion: func(output string) string {
			return strings.Replace(output, "Python ", "", 1)
		},
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

	c.UI.Info("Checking Trellis requirements...\n")

	c.UI.Info("Required:\n")

	requirementsMet := true

	for _, req := range Requirements {
		if req.Optional {
			continue
		}

		result, err := checkRequirement(req)
		if err != nil {
			c.UI.Error(err.Error())
		}

		if !result.Satisfied {
			requirementsMet = false
		}

		c.UI.Info(result.Message)
	}

	c.UI.Info("\nOptional:\n")

	for _, req := range Requirements {
		if !req.Optional {
			continue
		}

		result, err := checkRequirement(req)
		if err != nil {
			c.UI.Error(err.Error())
		}

		c.UI.Info(result.Message)
	}

	c.UI.Info("\nSee requirements documentation for more information:")
	c.UI.Info("https://roots.io/trellis/docs/installation/#requirements")

	if requirementsMet {
		return 0
	} else {
		return 1
	}
}

func (c *CheckCommand) Synopsis() string {
	return "Checks if the required and optional Trellis dependencies are installed"
}

func (c *CheckCommand) Help() string {
	helpText := `
Usage: trellis check

Checks if the required and optional Trellis dependencies are installed.

Options:
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}

func checkRequirement(req trellis.Requirement) (result trellis.RequirementResult, err error) {
	result, err = req.Check()
	if err != nil {
		return result, fmt.Errorf("Error checking %s requirement: %v", req.Name, err)
	}

	return result, nil
}
