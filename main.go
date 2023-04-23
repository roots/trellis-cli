package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/roots/trellis-cli/app_paths"
	"github.com/roots/trellis-cli/cmd"
	"github.com/roots/trellis-cli/github"
	"github.com/roots/trellis-cli/plugin"
	"github.com/roots/trellis-cli/trellis"
	"github.com/roots/trellis-cli/update"

	"github.com/fatih/color"
	"github.com/mitchellh/cli"
)

// To be replaced by goreleaser build flags.
var version = "canary"
var updaterRepo = ""
var experimentalCommands = []string{
	"vm",
}

func main() {
	c := cli.NewCLI("trellis", version)
	c.Args = os.Args[1:]

	ui := &cli.ColoredUi{
		ErrorColor: cli.UiColorRed,
		WarnColor:  cli.UiColor{Code: int(color.FgYellow), Bold: false},
		Ui: &cli.BasicUi{
			Reader:      os.Stdin,
			Writer:      os.Stdout,
			ErrorWriter: os.Stderr,
		},
	}

	trellis := trellis.NewTrellis()

	if err := trellis.LoadGlobalCliConfig(); err != nil {
		ui.Error(err.Error())
		os.Exit(1)
	}

	updateNotifier := &update.Notifier{
		CacheDir:  app_paths.CacheDir(),
		Client:    github.Client,
		Repo:      updaterRepo,
		SkipCheck: !trellis.CliConfig.CheckForUpdates,
		Version:   version,
	}

	updateMessageChan := make(chan *github.Release)
	go func() {
		release, _ := updateNotifier.CheckForUpdate()
		updateMessageChan <- release
	}()

	c.Commands = map[string]cli.CommandFactory{
		"alias": func() (cli.Command, error) {
			return cmd.NewAliasCommand(ui, trellis), nil
		},
		"check": func() (cli.Command, error) {
			return &cmd.CheckCommand{UI: ui, Trellis: trellis}, nil
		},
		"db": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				HelpText:     "Usage: trellis db <subcommand> [<args>]",
				SynopsisText: "Commands for database management",
			}, nil
		},
		"db open": func() (cli.Command, error) {
			return cmd.NewDBOpenCommand(ui, trellis), nil
		},
		"deploy": func() (cli.Command, error) {
			return cmd.NewDeployCommand(ui, trellis), nil
		},
		"dotenv": func() (cli.Command, error) {
			return cmd.NewDotEnvCommand(ui, trellis), nil
		},
		"down": func() (cli.Command, error) {
			return &cmd.DownCommand{UI: ui, Trellis: trellis}, nil
		},
		"droplet": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				HelpText:     "Usage: trellis droplet <subcommand> [<args>]",
				SynopsisText: "Commands for DigitalOcean Droplets",
			}, nil
		},
		"droplet create": func() (cli.Command, error) {
			return cmd.NewDropletCreateCommand(ui, trellis), nil
		},
		"droplet dns": func() (cli.Command, error) {
			return cmd.NewDropletDnsCommand(ui, trellis), nil
		},
		"exec": func() (cli.Command, error) {
			return &cmd.ExecCommand{UI: ui, Trellis: trellis}, nil
		},
		"galaxy": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				HelpText:     "Usage: trellis galaxy <subcommand> [<args>]",
				SynopsisText: "Commands for Ansible Galaxy",
			}, nil
		},
		"galaxy install": func() (cli.Command, error) {
			return &cmd.GalaxyInstallCommand{UI: ui, Trellis: trellis}, nil
		},
		"info": func() (cli.Command, error) {
			return &cmd.InfoCommand{UI: ui, Trellis: trellis}, nil
		},
		"init": func() (cli.Command, error) {
			return cmd.NewInitCommand(ui, trellis), nil
		},
		"key": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				HelpText:     "Usage: trellis key <subcommand> [<args>]",
				SynopsisText: "Commands for managing SSH keys",
			}, nil
		},
		"key generate": func() (cli.Command, error) {
			return cmd.NewKeyGenerateCommand(ui, trellis), nil
		},
		"logs": func() (cli.Command, error) {
			return cmd.NewLogsCommand(ui, trellis), nil
		},
		"new": func() (cli.Command, error) {
			return cmd.NewNewCommand(ui, trellis, c.Version), nil
		},
		"open": func() (cli.Command, error) {
			return &cmd.OpenCommand{UI: ui, Trellis: trellis}, nil
		},
		"provision": func() (cli.Command, error) {
			return cmd.NewProvisionCommand(ui, trellis), nil
		},
		"rollback": func() (cli.Command, error) {
			return cmd.NewRollbackCommand(ui, trellis), nil
		},
		"shell-init": func() (cli.Command, error) {
			return &cmd.ShellInitCommand{UI: ui}, nil
		},
		"ssh": func() (cli.Command, error) {
			return cmd.NewSshCommand(ui, trellis), nil
		},
		"up": func() (cli.Command, error) {
			return cmd.NewUpCommand(ui, trellis), nil
		},
		"vault": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				HelpText:     "Usage: trellis vault <subcommand> [<args>]",
				SynopsisText: "Commands for Ansible Vault",
			}, nil
		},
		"vault edit": func() (cli.Command, error) {
			return cmd.NewVaultEditCommand(ui, trellis), nil
		},
		"vault encrypt": func() (cli.Command, error) {
			return cmd.NewVaultEncryptCommand(ui, trellis), nil
		},
		"vault decrypt": func() (cli.Command, error) {
			return cmd.NewVaultDecryptCommand(ui, trellis), nil
		},
		"vault view": func() (cli.Command, error) {
			return cmd.NewVaultViewCommand(ui, trellis), nil
		},
		"valet": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				HelpText:     "Usage: trellis valet <subcommand> [<args>]",
				SynopsisText: "Commands for Laravel Valet",
			}, nil
		},
		"valet link": func() (cli.Command, error) {
			return &cmd.ValetLinkCommand{UI: ui, Trellis: trellis}, nil
		},
		"venv hook": func() (cli.Command, error) {
			return &cmd.VenvHookCommand{UI: ui, Trellis: trellis}, nil
		},
		"vm": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				HelpText:     "Usage: trellis vm <subcommand> [<args>]",
				SynopsisText: "Commands for managing development virtual machines",
			}, nil
		},
		"vm delete": func() (cli.Command, error) {
			return cmd.NewVmDeleteCommand(ui, trellis), nil
		},
		"vm shell": func() (cli.Command, error) {
			return cmd.NewVmShellCommand(ui, trellis), nil
		},
		"vm start": func() (cli.Command, error) {
			return cmd.NewVmStartCommand(ui, trellis), nil
		},
		"vm stop": func() (cli.Command, error) {
			return cmd.NewVmStopCommand(ui, trellis), nil
		},
		"vm sudoers": func() (cli.Command, error) {
			return &cmd.VmSudoersCommand{UI: ui, Trellis: trellis}, nil
		},
		"xdebug-tunnel": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				HelpText:     "Usage: trellis xdebug-tunnel <subcommand> [<args>]",
				SynopsisText: "Commands for Xdebug tunnel",
			}, nil
		},
		"xdebug-tunnel open": func() (cli.Command, error) {
			return cmd.NewXdebugTunnelOpenCommand(ui, trellis), nil
		},
		"xdebug-tunnel close": func() (cli.Command, error) {
			return cmd.NewXdebugTunnelCloseCommand(ui, trellis), nil
		},
	}

	c.HiddenCommands = []string{"venv", "venv hook"}
	c.HelpFunc = experimentalCommandHelpFunc(c.Name, cli.BasicHelpFunc("trellis"))

	if trellis.CliConfig.LoadPlugins {
		pluginPaths := filepath.SplitList(os.Getenv("PATH"))
		plugin.Register(c, pluginPaths, []string{"trellis"})
	}

	exitStatus, err := c.Run()

	if err != nil {
		ui.Error(err.Error())
	}

	newRelease := <-updateMessageChan
	if newRelease != nil {
		msg := fmt.Sprintf(
			"\n%s %s → %s\n%s",
			color.YellowString("A new release of trellis-cli is available:"),
			color.CyanString(version),
			color.CyanString(newRelease.Version),
			color.YellowString(newRelease.URL),
		)

		ui.Info(msg)
	}

	os.Exit(exitStatus)
}
