package main

import (
	"github.com/mitchellh/cli"
	"log"
	"os"
	"trellis-cli/cmd"
	"trellis-cli/trellis"
)

func main() {
	c := cli.NewCLI("trellis", "0.1.0")
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
		"deploy": func() (cli.Command, error) {
			return &cmd.DeployCommand{UI: ui, Trellis: trellis}, nil
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
		"new": func() (cli.Command, error) {
			return cmd.NewNewCommand(ui, trellis), nil
		},
		"provision": func() (cli.Command, error) {
			return cmd.NewProvisionCommand(ui, trellis), nil
		},
		"rollback": func() (cli.Command, error) {
			return cmd.NewRollbackCommand(ui, trellis), nil
		},
	}

	exitStatus, err := c.Run()

	if err != nil {
		log.Println(err)
	}

	os.Exit(exitStatus)
}
