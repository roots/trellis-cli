package cmd

import (
	"fmt"
	"github.com/mitchellh/cli"
	"os/exec"
	"strings"
)

var execCommand = exec.Command

type UiErrorWriter struct {
	Ui cli.Ui
}

func (w *UiErrorWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	if n > 0 && p[n-1] == '\n' {
		p = p[:n-1]
	}

	w.Ui.Error(string(p))
	return n, nil
}

func logCmd(cmd *exec.Cmd, ui cli.Ui, output bool) {
	cmd.Stderr = &UiErrorWriter{ui}

	if output {
		cmd.Stdout = &cli.UiWriter{ui}
	}

	fmt.Println("Running command =>", strings.Join(cmd.Args, " "))
}
