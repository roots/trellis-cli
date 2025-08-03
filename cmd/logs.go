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

	sshHost := c.Trellis.SshHost(environment, siteName, "web")

	_, err := exec.LookPath("goaccess")

	if (c.goaccess || c.goaccessFlags != "") && err == nil {
		tailCmd := c.tailCmd(siteName, "goaccess")
		logArgs := []string{sshHost, tailCmd}

		ssh := command.Cmd("ssh", logArgs)
		goaccessArgs := []string{"--log-format=COMBINED"}

		if c.goaccessFlags != "" {
			goaccessArgs = append(goaccessArgs, strings.Split(c.goaccessFlags, " ")...)
		}

		goaccess := command.WithOptions(
			command.WithTermOutput(),
		).Cmd("goaccess", goaccessArgs)

		goaccess.Stdin, _ = ssh.StdoutPipe()

		if err := ssh.Start(); err != nil {
			c.UI.Error(fmt.Sprintf("Error starting SSH command: %s", err))
			return 1
		}

		if err := goaccess.Start(); err != nil {
			c.UI.Error(fmt.Sprintf("Error starting goaccess command: %s", err))
			return 1
		}

		if err := goaccess.Wait(); err != nil {
			c.UI.Error(fmt.Sprintf("Error running goaccess command: %s", err))
			return 1
		}
		if err := ssh.Wait(); err != nil {
			c.UI.Error(fmt.Sprintf("Error running SSH command: %s", err))
			return 1
		}
	} else {
		logArgs := []string{sshHost, c.tailCmd(siteName, "tail")}

		ssh := command.WithOptions(
			command.WithTermOutput(),
			command.WithLogging(c.UI),
		).Cmd("ssh", logArgs)

		if err := ssh.Run(); err != nil {
			c.UI.Error(fmt.Sprintf("Error running ssh: %s", err))
			return 1
		}
	}

	return 0
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
