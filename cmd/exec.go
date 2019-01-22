package cmd

import (
	"fmt"
	"github.com/mitchellh/cli"
	"os"
	"os/exec"
	"strings"
)

func logCmd(cmd *exec.Cmd, ui cli.Ui, output bool) {
	cmd.Stderr = os.Stderr

	if output {
		cmd.Stdout = &cli.UiWriter{ui}
	}

	fmt.Println("Running command =>", strings.Join(cmd.Args, " "))
}
