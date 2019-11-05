package cmd

import (
	"flag"
	"fmt"
	"github.com/fatih/color"
	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	. "trellis-cli/templates"
	"trellis-cli/trellis"
)

type AliasCommand struct {
	UI      cli.Ui
	flags   *flag.FlagSet
	Trellis *trellis.Trellis
	local   string
}

func NewAliasCommand(ui cli.Ui, trellis *trellis.Trellis) *AliasCommand {
	c := &AliasCommand{UI: ui, Trellis: trellis}
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

	// Template playbook files to Trellis
	files := map[string]string{
		"alias.yml":      ALIAS_YML,
		"alias.yml.j2":   ALIAS_YML_J2,
		"alias-copy.yml": ALIAS_COPY_YML,
	}
	for filename, fileContent := range files {
		writeFile(filename, TrimSpace(fileContent))
		defer deleteFile(filename)
	}

	environments := c.Trellis.EnvironmentNames()
	var remoteEnvironments []string
	for _, environment := range environments {
		if environment != c.local {
			remoteEnvironments = append(remoteEnvironments, environment)
		}
	}

	tempDir, tempDirErr := ioutil.TempDir("", "trellis-alias-")
	if tempDirErr != nil {
		log.Fatal(tempDirErr)
	}
	defer os.RemoveAll(tempDir)

	for _, environment := range remoteEnvironments {
		alias := execCommand("ansible-playbook", "alias.yml", "-vvv", "-e", "env="+environment, "-e", "trellis_alias_j2=alias.yml.j2", "-e", "trellis_alias_temp_dir="+tempDir)
		appendEnvironmentVariable(alias, "ANSIBLE_RETRY_FILES_ENABLED=false")

		logCmd(alias, c.UI, true)
		err := alias.Run()

		if err != nil {
			c.UI.Error(fmt.Sprintf("Error running ansible-playbook alias.yml: %s", err))
			return 1
		}
	}

	combined := ""
	for _, environment := range remoteEnvironments {
		part, err := ioutil.ReadFile(filepath.Join(tempDir, environment+".yml.part"))
		if err != nil {
			log.Fatal(err)
		}
		combined = combined + string(part)
	}

	combinedYmlPath := filepath.Join(tempDir, "/combined.yml")
	writeFileErr := ioutil.WriteFile(combinedYmlPath, []byte(combined), 0644)
	if writeFileErr != nil {
		log.Fatal(writeFileErr)
	}

	aliasCopy := execCommand("ansible-playbook", "alias-copy.yml", "-e", "env="+c.local, "-e", "trellis_alias_combined="+combinedYmlPath)
	appendEnvironmentVariable(aliasCopy, "ANSIBLE_RETRY_FILES_ENABLED=false")

	logCmd(aliasCopy, c.UI, true)
	aliasCopyErr := aliasCopy.Run()

	if aliasCopyErr != nil {
		c.UI.Error(fmt.Sprintf("Error running ansible-playbook alias-copy.yml: %s", aliasCopyErr))
		return 1
	}

	c.UI.Info(color.GreenString("✓ wp-cli.trellis-alias.yml generated"))
	message := `
Action Required: Add these lines into wp-cli.yml or wp-cli.local.yml

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
      --local (default: development) Local environment name
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}

func (c *AliasCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--local": complete.PredictNothing,
	}
}
