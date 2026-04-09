package cmd

import (
	"flag"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/wsl"
	"github.com/roots/trellis-cli/trellis"

	"github.com/hashicorp/cli"
)

type VmOpenCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
}

func NewVmOpenCommand(ui cli.Ui, trellis *trellis.Trellis) *VmOpenCommand {
	c := &VmOpenCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VmOpenCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
}

func (c *VmOpenCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	if err := commandArgumentValidator.validate(args); err != nil {
		c.UI.Error(err.Error())
		c.UI.Output(c.Help())
		return 1
	}

	if windowsHostRequired(c.Trellis, c.UI, "vm open") {
		return 1
	}

	if runtime.GOOS != "windows" {
		c.UI.Error("'trellis vm open' is only supported on Windows (WSL2).")
		c.UI.Info("On macOS/Linux, open your site directory directly in your editor.")
		return 1
	}

	instanceName, err := c.Trellis.GetVmInstanceName()
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	distro := "trellis-" + strings.ReplaceAll(instanceName, ".", "-")

	// Warn if the distro isn't running — VS Code's WSL extension will
	// silently boot it, but services (nginx, php-fpm, mariadb) won't
	// be started. The developer likely wants `vm start` first.
	if output, err := command.Cmd("wsl", []string{"-l", "--running", "-q"}).Output(); err == nil {
		running := false
		decoded := wsl.DecodeWslOutput(output)
		for _, line := range strings.Split(decoded, "\n") {
			if strings.TrimSpace(line) == distro {
				running = true
				break
			}
		}
		if !running {
			c.UI.Warn("VM is not running. Web services (nginx, PHP, MariaDB) won't be available.")
			c.UI.Warn("Run 'trellis vm start' first for the full development environment.\n")

			prompt := promptui.Prompt{
				Label:     "Open VS Code anyway",
				IsConfirm: true,
			}

			if _, err := prompt.Run(); err != nil {
				c.UI.Info("Aborted.")
				return 0
			}
		}
	}

	// The full project (trellis/ + site/ + .git/) lives on ext4 at
	// /home/admin/<project>/. Open VS Code at the project root so the
	// developer sees the familiar layout and can use git normally.
	projectName := filepath.Base(filepath.Dir(c.Trellis.Path))
	remotePath := fmt.Sprintf("/home/admin/%s", projectName)

	// VS Code's --folder-uri flag opens a folder inside a WSL distro.
	// The vscode-remote URI format is: vscode-remote://wsl+<distro>/<path>
	c.UI.Info(fmt.Sprintf("Opening VS Code in WSL distro '%s' at %s...", distro, remotePath))

	folderURI := fmt.Sprintf("vscode-remote://wsl+%s%s", distro, remotePath)
	cmd := exec.Command("code", "--folder-uri", folderURI)
	if err := cmd.Run(); err != nil {
		c.UI.Error(fmt.Sprintf("Could not open VS Code: %v", err))
		c.UI.Info("Make sure VS Code is installed and the 'code' command is in your PATH.")
		c.UI.Info(fmt.Sprintf("You can also open VS Code manually and connect to WSL distro '%s'", distro))
		return 1
	}

	return 0
}

func (c *VmOpenCommand) Synopsis() string {
	return "Opens VS Code in the VM's project directory (Windows/WSL2)"
}

func (c *VmOpenCommand) Help() string {
	helpText := `
Usage: trellis vm open [options]

Opens VS Code connected to the WSL2 distro at the project root.

Your project (trellis/ + site/ + .git/) is copied to the WSL2 ext4
filesystem during 'trellis vm start' for optimal performance. This
command opens VS Code at that copy so you can edit files and use
git as normal.

Note: Requires VS Code with the WSL extension installed.
Do NOT edit files on the Windows side after 'vm start' — the WSL
copy is your working directory.

Options:
  -h, --help  Show this help
`

	return strings.TrimSpace(helpText)
}
