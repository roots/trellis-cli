package cmd

import (
	_ "embed"
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/ansible"
	"github.com/roots/trellis-cli/trellis"
)

type AliasCommand struct {
	UI                cli.Ui
	flags             *flag.FlagSet
	Trellis           *trellis.Trellis
	local             string
	skipLocal         bool
	aliasPlaybook     *AdHocPlaybook
	aliasCopyPlaybook *AdHocPlaybook
}

//go:embed files/playbooks/alias.yml
var aliasYml string

//go:embed files/playbooks/alias_template.yml.j2
var aliasYmlJ2 string

//go:embed files/playbooks/alias_copy.yml
var aliasCopyYml string

func NewAliasCommand(ui cli.Ui, trellis *trellis.Trellis) *AliasCommand {
	aliasPlaybook := &AdHocPlaybook{
		path: trellis.Path,
		files: map[string]string{
			"alias.yml":    aliasYml,
			"alias.yml.j2": aliasYmlJ2,
		},
	}

	aliasCopyPlaybook := &AdHocPlaybook{
		path: trellis.Path,
		files: map[string]string{
			"alias-copy.yml": aliasCopyYml,
		},
	}

	c := &AliasCommand{UI: ui, Trellis: trellis, aliasPlaybook: aliasPlaybook, aliasCopyPlaybook: aliasCopyPlaybook}
	c.init()
	return c
}

func (c *AliasCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.local, "local", "development", "Local environment name (default: development)")
	c.flags.BoolVar(&c.skipLocal, "skip-local", false, "Skip local environment in aliases (default: false)")
}

func (c *AliasCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	spinner := NewSpinner(
		SpinnerCfg{
			Message:     "Generating WP-CLI aliases config",
			StopMessage: "wp-cli.trellis-alias.yml generated",
			FailMessage: "Error generating config",
		},
	)
	_ = spinner.Start()

	environments := c.Trellis.EnvironmentNames()
	var envsToAlias []string
	for _, environment := range environments {
		if c.skipLocal && environment == c.local {
			continue
		}

		envsToAlias = append(envsToAlias, environment)
	}

	tempDir, tempDirErr := os.MkdirTemp("", "trellis-alias-")
	if tempDirErr != nil {
		_ = spinner.StopFail()
		c.UI.Error(tempDirErr.Error())
		return 1
	}
	defer os.RemoveAll(tempDir)

	defer c.aliasPlaybook.DumpFiles()()

	devInventory := findDevInventory(c.Trellis, c.UI)

	for _, environment := range envsToAlias {
		playbook := ansible.Playbook{
			Name:    "alias.yml",
			Verbose: true,
			Env:     environment,
			ExtraVars: map[string]string{
				"trellis_alias_j2":       "alias.yml.j2",
				"trellis_alias_temp_dir": tempDir,
			},
		}

		if !c.skipLocal && c.local == environment {
			_, site, err := c.Trellis.MainSiteFromEnvironment(environment)

			if err != nil {
				_ = spinner.StopFail()
				c.UI.Error(err.Error())
				return 1
			}

			playbook.SetInventory(devInventory)
			playbook.AddExtraVar("include_local_env", "true")
			playbook.AddExtraVar("local_hostname_alias", site.MainHost())
		}

		mockUi := cli.NewMockUi()
		aliasPlaybook := command.WithOptions(
			command.WithUiOutput(mockUi),
		).Cmd("ansible-playbook", playbook.CmdArgs())

		if err := aliasPlaybook.Run(); err != nil {
			_ = spinner.StopFail()
			c.UI.Error("Error creating WP-CLI aliases. Temporary playbook failed to execute:")
			c.UI.Error(mockUi.ErrorWriter.String())
			return 1
		}
	}

	combined := ""
	for _, environment := range envsToAlias {
		part, err := os.ReadFile(filepath.Join(tempDir, environment+".yml.part"))
		if err != nil {
			_ = spinner.StopFail()
			c.UI.Error(err.Error())
			return 1
		}
		combined = combined + string(part)
	}

	combinedYmlPath := filepath.Join(tempDir, "/combined.yml")
	writeFileErr := os.WriteFile(combinedYmlPath, []byte(combined), 0644)
	if writeFileErr != nil {
		_ = spinner.StopFail()
		c.UI.Error(writeFileErr.Error())
		return 1
	}

	defer c.aliasCopyPlaybook.DumpFiles()()

	playbook := ansible.Playbook{
		Name: "alias-copy.yml",
		Env:  c.local,
		ExtraVars: map[string]string{
			"trellis_alias_combined": combinedYmlPath,
		},
	}

	mockUi := cli.NewMockUi()
	aliasCopyPlaybook := command.WithOptions(
		command.WithUiOutput(mockUi),
	).Cmd("ansible-playbook", playbook.CmdArgs())

	if err := aliasCopyPlaybook.Run(); err != nil {
		_ = spinner.StopFail()
		c.UI.Error("Error creating WP-CLI aliases. Temporary playbook failed to execute:")
		c.UI.Error(mockUi.ErrorWriter.String())
		return 1
	}

	_ = spinner.Stop()
	c.UI.Info("")
	message := `
Action Required: use the generated config by adding these lines to site/wp-cli.yml or an alternative wp-cli.yml (or wp-cli.local.yml) config. 

_: 
  inherit: wp-cli.trellis-alias.yml
`
	c.UI.Info(strings.TrimSpace(message))

	return 0
}

func (c *AliasCommand) Synopsis() string {
	return "Generate WP CLI aliases for remote environments"
}

func (c *AliasCommand) Help() string {
	helpText := `
Usage: trellis alias [options]

Generate WP CLI aliases for remote environments

Don't include local env (useful for non-Lima users):

  $ trellis alias --skip-local

Options:
      --local         Local environment name (default: development)
      --skip-local    Skip local environment in aliases (default: false)
  -h, --help          Show this help
`

	return strings.TrimSpace(helpText)
}

func (c *AliasCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *AliasCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--local":      predictEnvironment(c.Trellis),
		"--skip-local": complete.PredictNothing,
	}
}

func predictEnvironment(t *trellis.Trellis) complete.PredictFunc {
	return func(args complete.Args) []string {
		if err := t.LoadProject(); err != nil {
			return []string{}
		}

		return t.EnvironmentNames()
	}
}
