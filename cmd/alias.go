package cmd

import (
	_ "embed"
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
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
	spinner.Start()

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
		spinner.StopFail()
		c.UI.Error(tempDirErr.Error())
		return 1
	}
	defer os.RemoveAll(tempDir)

	defer c.aliasPlaybook.DumpFiles()()

	for _, environment := range envsToAlias {
		args := []string{
			"alias.yml",
			"-vvv",
			"-e", "env=" + environment,
			"-e", "trellis_alias_j2=alias.yml.j2",
			"-e", "trellis_alias_temp_dir=" + tempDir,
		}

		if !c.skipLocal && c.local == environment {
			siteName, _ := c.Trellis.FindSiteNameFromEnvironment(environment, "")
			mainHost := c.Trellis.SiteFromEnvironmentAndName(environment, siteName).MainHost()
			args = append(args, "-e", "include_local_env=true")
			args = append(args, "-e", "local_hostname_alias="+mainHost)
		}

		mockUi := cli.NewMockUi()
		aliasPlaybook := command.WithOptions(
			command.WithUiOutput(mockUi),
		).Cmd("ansible-playbook", args)

		if err := aliasPlaybook.Run(); err != nil {
			spinner.StopFail()
			c.UI.Error("Error creating WP-CLI aliases. Temporary playbook failed to execute:")
			c.UI.Error(mockUi.ErrorWriter.String())
			return 1
		}
	}

	combined := ""
	for _, environment := range envsToAlias {
		part, err := os.ReadFile(filepath.Join(tempDir, environment+".yml.part"))
		if err != nil {
			spinner.StopFail()
			c.UI.Error(err.Error())
			return 1
		}
		combined = combined + string(part)
	}

	combinedYmlPath := filepath.Join(tempDir, "/combined.yml")
	writeFileErr := os.WriteFile(combinedYmlPath, []byte(combined), 0644)
	if writeFileErr != nil {
		spinner.StopFail()
		c.UI.Error(writeFileErr.Error())
		return 1
	}

	defer c.aliasCopyPlaybook.DumpFiles()()

	mockUi := cli.NewMockUi()
	aliasCopyPlaybook := command.WithOptions(
		command.WithUiOutput(mockUi),
	).Cmd("ansible-playbook", []string{"alias-copy.yml", "-e", "env=" + c.local, "-e", "trellis_alias_combined=" + combinedYmlPath})

	if err := aliasCopyPlaybook.Run(); err != nil {
		spinner.StopFail()
		c.UI.Error("Error creating WP-CLI aliases. Temporary playbook failed to execute:")
		c.UI.Error(mockUi.ErrorWriter.String())
		return 1
	}

	spinner.Stop()
	c.UI.Info("")
	message := `
Action Required: use the generated config by adding these lines to your wp-cli.yml or wp-cli.local.yml config.

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

Don't include local env (useful for non-Vagrant users):

  $ trellis alias --skip-local

Options:
      --local         Local environment name (default: development)
      --skip-local    Skip local environment in aliases (default: false)
  -h, --help          Show this help
`

	return strings.TrimSpace(helpText)
}

func (c *AliasCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--local":      complete.PredictNothing,
		"--skip-local": complete.PredictNothing,
	}
}
