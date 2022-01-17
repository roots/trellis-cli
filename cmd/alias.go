package cmd

import (
	_ "embed"
	"flag"
	"fmt"
	"io/ioutil"
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
	aliasPlaybook     *AdHocPlaybook
	aliasCopyPlaybook *AdHocPlaybook
}

//go:embed files/playbooks/alias.yml
var aliasYml string

const aliasYmlJ2 = `
@{{ env }}:
  ssh: "{{ web_user }}@{{ ansible_host }}:{{ ansible_port | default('22') }}"
  path: "{{ project_root | default(www_root + '/' + item.key) | regex_replace('^~\/','') }}/{{ item.current_path | default('current') }}/web/wp"
`

//go:embed files/playbooks/alias_copy.yml
var aliasCopyYml string

func NewAliasCommand(ui cli.Ui, trellis *trellis.Trellis) *AliasCommand {
	aliasPlaybook := &AdHocPlaybook{
		path: trellis.Path,
		files: map[string]string{
			"alias.yml":    aliasYml,
			"alias.yml.j2": strings.TrimSpace(aliasYmlJ2) + "\n",
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
	c.flags.StringVar(&c.local, "local", "development", "local environment name, default: development")
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
	var remoteEnvironments []string
	for _, environment := range environments {
		if environment != c.local {
			remoteEnvironments = append(remoteEnvironments, environment)
		}
	}

	tempDir, tempDirErr := ioutil.TempDir("", "trellis-alias-")
	if tempDirErr != nil {
		spinner.StopFail()
		c.UI.Error(tempDirErr.Error())
		return 1
	}
	defer os.RemoveAll(tempDir)

	defer c.aliasPlaybook.DumpFiles()()

	for _, environment := range remoteEnvironments {
		args := []string{
			"alias.yml",
			"-vvv",
			"-e", "env=" + environment,
			"-e", "trellis_alias_j2=alias.yml.j2",
			"-e", "trellis_alias_temp_dir=" + tempDir,
		}
		aliasPlaybook := command.Cmd("ansible-playbook", args)

		if err := aliasPlaybook.Run(); err != nil {
			spinner.StopFail()
			c.UI.Error(fmt.Sprintf("Error running ansible-playbook alias.yml: %s", err))
			return 1
		}
	}

	combined := ""
	for _, environment := range remoteEnvironments {
		part, err := ioutil.ReadFile(filepath.Join(tempDir, environment+".yml.part"))
		if err != nil {
			spinner.StopFail()
			c.UI.Error(err.Error())
			return 1
		}
		combined = combined + string(part)
	}

	combinedYmlPath := filepath.Join(tempDir, "/combined.yml")
	writeFileErr := ioutil.WriteFile(combinedYmlPath, []byte(combined), 0644)
	if writeFileErr != nil {
		spinner.StopFail()
		c.UI.Error(writeFileErr.Error())
		return 1
	}

	defer c.aliasCopyPlaybook.DumpFiles()()

	aliasCopyPlaybook := command.Cmd("ansible-playbook", []string{"alias-copy.yml", "-e", "env=" + c.local, "-e", "trellis_alias_combined=" + combinedYmlPath})

	if err := aliasCopyPlaybook.Run(); err != nil {
		spinner.StopFail()
		c.UI.Error(fmt.Sprintf("Error running ansible-playbook alias-copy.yml: %s", err))
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

Options:
      --local Local environment name (default: development)
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}

func (c *AliasCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--local": complete.PredictNothing,
	}
}
