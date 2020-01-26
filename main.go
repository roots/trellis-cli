package main

import (
	"log"
	"os"
	"trellis-cli/cmd"
	"trellis-cli/trellis"

	"github.com/mitchellh/cli"
)

func main() {
	c := cli.NewCLI("trellis", "0.7.1")
	c.Args = os.Args[1:]

	ui := &cli.ColoredUi{
		ErrorColor: cli.UiColorRed,
		Ui: &cli.BasicUi{
			Reader:      os.Stdin,
			Writer:      os.Stdout,
			ErrorWriter: os.Stderr,
		},
	}

	project := &trellis.Project{}
	trellis := trellis.NewTrellis(project)

	c.Commands = map[string]cli.CommandFactory{
		"alias": func() (cli.Command, error) {
			return cmd.NewAliasCommand(ui, trellis), nil
		},
		"check": func() (cli.Command, error) {
			return &cmd.CheckCommand{UI: ui, Trellis: trellis}, nil
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
			return &cmd.DropletCommand{UI: ui, Trellis: trellis}, nil
		},
		"droplet create": func() (cli.Command, error) {
			return cmd.NewDropletCreateCommand(ui, trellis), nil
		},
		"exec": func() (cli.Command, error) {
			return &cmd.ExecCommand{UI: ui, Trellis: trellis}, nil
		},
		"galaxy": func() (cli.Command, error) {
			return &cmd.GalaxyCommand{UI: ui, Trellis: trellis}, nil
		},
		"galaxy install": func() (cli.Command, error) {
			return &cmd.GalaxyInstallCommand{UI: ui, Trellis: trellis}, nil
		},
		"info": func() (cli.Command, error) {
			return &cmd.InfoCommand{UI: ui, Trellis: trellis}, nil
		},
		"init": func() (cli.Command, error) {
			return &cmd.InitCommand{UI: ui, Trellis: trellis}, nil
		},
		"new": func() (cli.Command, error) {
			return cmd.NewNewCommand(ui, trellis, c.Version), nil
		},
		"provision": func() (cli.Command, error) {
			return cmd.NewProvisionCommand(ui, trellis), nil
		},
		"rollback": func() (cli.Command, error) {
			return cmd.NewRollbackCommand(ui, trellis), nil
		},
		"ssh": func() (cli.Command, error) {
			return &cmd.SshCommand{ui, trellis, &cmd.SyscallCommandExecutor{}}, nil
		},
		"up": func() (cli.Command, error) {
			return cmd.NewUpCommand(ui, trellis), nil
		},
		"vault": func() (cli.Command, error) {
			return &cmd.VaultCommand{UI: ui, Trellis: trellis}, nil
		},
		"vault edit": func() (cli.Command, error) {
			return cmd.NewVaultEditCommand(ui, trellis, &cmd.SyscallCommandExecutor{}), nil
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
			return &cmd.ValetCommand{UI: ui, Trellis: trellis}, nil
		},
		"valet link": func() (cli.Command, error) {
			return &cmd.ValetLinkCommand{UI: ui, Trellis: trellis}, nil
		},
	}

	exitStatus, err := c.Run()

	if err != nil {
		log.Println(err)
	}

	os.Exit(exitStatus)
}
