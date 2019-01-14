package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func logCmd(cmd *exec.Cmd, output bool) {
	cmd.Stderr = os.Stderr

	if output {
		cmd.Stdout = os.Stdout
	}

	fmt.Println("Running command =>", strings.Join(cmd.Args, " "))
}
