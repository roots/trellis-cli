package cmd

import (
	"flag"
	"fmt"
	"os/exec"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

func NewLogsCommand(ui cli.Ui, trellis *trellis.Trellis) *LogsCommand {
	c := &LogsCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

type LogsCommand struct {
	UI            cli.Ui
	flags         *flag.FlagSet
	access        bool
	error         bool
	goaccess      bool
	goaccessFlags string
	number        string
	Trellis       *trellis.Trellis
}

func (c *LogsCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.access, "access", false, "Show access logs only")
	c.flags.BoolVar(&c.error, "error", false, "Show error logs only")
	c.flags.StringVar(&c.goaccessFlags, "goaccess-flags", "", "Flags to pass to the goaccess command (in quotes)")
	c.flags.BoolVar(&c.goaccess, "g", false, "Uses goaccess as the log viewer instead of tail")
	c.flags.BoolVar(&c.goaccess, "goaccess", false, "Uses goaccess as the log viewer instead of tail")
	c.flags.StringVar(&c.number, "n", "", "Location (number lines) corresponding to tail's '-n' option")
	c.flags.StringVar(&c.number, "number", "", "Location (number lines) corresponding to tail's '-n' option")
}

func (c *LogsCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 1, optional: 1}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	environment := args[0]
	environmentErr := c.Trellis.ValidateEnvironment(environment)
	if environmentErr != nil {
		c.UI.Error(environmentErr.Error())
		return 1
	}

	siteNameArg := ""
	if len(args) == 2 {
		siteNameArg = args[1]
	}
	siteName, siteNameErr := c.Trellis.FindSiteNameFromEnvironment(environment, siteNameArg)
	if siteNameErr != nil {
		c.UI.Error(siteNameErr.Error())
		return 1
	}

	if c.useGoaccess() {
		if _, err := exec.LookPath("goaccess"); err != nil {
			c.UI.Error("goaccess command not found. Install it to use --goaccess flag: https://goaccess.io/")
			return 1
		}
	}

	if environment == "development" && c.Trellis.VmManagerType() != "" {
		return c.runWithVm(siteName)
	}

	sshHost := c.Trellis.SshHost(environment, siteName, "web")

	if c.useGoaccess() {
		tailCmd := c.tailCmd(siteName, "goaccess")
		ssh := command.Cmd("ssh", []string{sshHost, tailCmd})
		return c.runGoaccess(ssh, "SSH")
	}

	logArgs := []string{sshHost, c.tailCmd(siteName, "tail")}

	ssh := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(c.UI),
	).Cmd("ssh", logArgs)

	if err := ssh.Run(); err != nil {
		c.UI.Error(fmt.Sprintf("Error running ssh: %s", err))
		return 1
	}

	return 0
}

func (c *LogsCommand) runWithVm(siteName string) int {
	manager, err := newVmManager(c.Trellis, c.UI)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if c.useGoaccess() {
		return c.runWithVmGoaccess(manager, siteName)
	}

	tailCmd := c.tailCmd(siteName, "tail")
	args := []string{"bash", "-c", tailCmd}

	if err := manager.RunCommand(args, ""); err != nil {
		c.UI.Error(fmt.Sprintf("Error: %s", err))
		return 1
	}

	return 0
}

func (c *LogsCommand) runWithVmGoaccess(manager interface {
	RunCommandPipe([]string, string) (*exec.Cmd, error)
}, siteName string) int {
	tailCmd := c.tailCmd(siteName, "goaccess")
	args := []string{"bash", "-c", tailCmd}

	vmCmd, err := manager.RunCommandPipe(args, "")
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return c.runGoaccess(vmCmd, "VM")
}

func (c *LogsCommand) Synopsis() string {
	return "Tails the Nginx log files for an environment"
}

func (c *LogsCommand) Help() string {
	helpText := `
Usage: trellis logs [options] ENVIRONMENT [SITE]

Tails the Nginx log files for an environment.

Automatically integrates with https://goaccess.io/ when the --goaccess option is used.

Note: this command relies on an SSH connection to the environment's hostname to remotely
tail the log files. It depends on SSH keys being setup properly for a passwordless SSH connection.
If the 'trellis ssh' command does not work, this logs command won't work either.

View production logs:

  $ trellis logs production

View access logs only:

  $ trellis logs --access production

View error logs only:

  $ trellis logs --error production

View logs in goaccess:

  $ trellis logs --goaccess production

Pass flags to goaccess:

  $ trellis logs --goaccess-flags="-a -m" production

View the last 50 log lines (-n corresponds to tail's -n option):

  $ trellis logs -n 50 production

Arguments:
  ENVIRONMENT Name of environment (ie: production)
  SITE        Name of site (ie: example.com)
  
Options:
      --access          Show access logs only
      --error           Show error logs only
  -g, --goaccess        Uses goaccess as the log viewer instead of tail
      --goaccess-flags  Flags to pass to the goaccess command (in quotes)
  -n, --number          Location (number lines) corresponding to tail's '-n' argument
  -h, --help            Show this help
`

	return strings.TrimSpace(helpText)
}

func (c *LogsCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteSite(c.flags)
}

func (c *LogsCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--access":         complete.PredictNothing,
		"--error":          complete.PredictNothing,
		"--goaccess":       complete.PredictNothing,
		"--goaccess-flags": complete.PredictNothing,
		"--number":         complete.PredictNothing,
	}
}

func (c *LogsCommand) useGoaccess() bool {
	return c.goaccess || c.goaccessFlags != ""
}

func (c *LogsCommand) goaccessArgs() []string {
	args := []string{"--log-format=COMBINED"}
	if c.goaccessFlags != "" {
		args = append(args, strings.Split(c.goaccessFlags, " ")...)
	}
	return args
}

func (c *LogsCommand) runGoaccess(sourceCmd *exec.Cmd, sourceName string) int {
	goaccess := command.WithOptions(
		command.WithTermOutput(),
	).Cmd("goaccess", c.goaccessArgs())

	goaccess.Stdin, _ = sourceCmd.StdoutPipe()

	if err := sourceCmd.Start(); err != nil {
		c.UI.Error(fmt.Sprintf("Error starting %s command: %s", sourceName, err))
		return 1
	}

	if err := goaccess.Start(); err != nil {
		c.UI.Error(fmt.Sprintf("Error starting goaccess: %s", err))
		return 1
	}

	_ = goaccess.Wait()
	_ = sourceCmd.Wait()

	return 0
}

func (c *LogsCommand) tailCmd(siteName string, mode string) string {
	file := "*[^gz]?" // exclude gzipped log files by default

	if c.access {
		file = "access.log"
	} else if c.error {
		file = "error.log"
	} else if mode == "goaccess" {
		// goaccess supports gzipped log files so we can include them
		file = "*"
	}

	n := ""
	if c.number == "" && mode == "goaccess" {
		// default to load entire file in Goaccess
		// this makes more sense in this context than for viewing in terminal
		n = "-n +0 "
	} else if c.number != "" {
		n = fmt.Sprintf("-n %s ", c.number)
	}

	return fmt.Sprintf("tail %s-f /srv/www/%s/logs/%s", n, siteName, file)
}
