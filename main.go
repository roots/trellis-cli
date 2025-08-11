package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/roots/trellis-cli/app_paths"
	"github.com/roots/trellis-cli/cmd"
	"github.com/roots/trellis-cli/github"
	"github.com/roots/trellis-cli/plugin"
	"github.com/roots/trellis-cli/trellis"
	"github.com/roots/trellis-cli/update"

	"github.com/fatih/color"
	"github.com/hashicorp/cli"
	"golang.org/x/term"
)

// To be replaced by goreleaser build flags.
var version = "canary"
var updaterRepo = ""
var deprecatedCommands = []string{
	"down",
	"up",
}

// Namespace commands and their subcommands
var namespaceCommands = map[string]map[string]string{
	"db": {
		"open": "Open database with GUI applications",
	},
	"droplet": {
		"create": "Creates a DigitalOcean Droplet server and provisions it",
		"dns":    "Creates DNS records for all WordPress sites' hosts in an environment",
	},
	"galaxy": {
		"install": "Installs Ansible Galaxy roles",
	},
	"key": {
		"generate": "Generates an SSH key",
	},
	"vault": {
		"edit":    "Opens vault file in editor",
		"encrypt": "Encrypts files with Ansible Vault",
		"decrypt": "Decrypts files with Ansible Vault",
		"view":    "Views vault encrypted file contents",
	},
	"valet": {
		"link": "Links a Trellis site for use with Laravel Valet",
	},
	"vm": {
		"delete":  "Deletes the development virtual machine",
		"shell":   "Executes shell in the VM",
		"start":   "Starts a development virtual machine",
		"stop":    "Stops the development virtual machine",
		"sudoers": "Generates sudoers content for passwordless updating of /etc/hosts",
	},
	"xdebug-tunnel": {
		"open":  "Opens a remote SSH tunnel to allow remote Xdebug connections",
		"close": "Closes the remote SSH Xdebug tunnel",
	},
}

// Global flag to track help requests
var showHelpFor string

func preprocessArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}

	// Check for help requests and remove --help from args
	newArgs := []string{}
	for i, arg := range args {
		if arg == "--help" || arg == "-h" {
			// Set help flag based on command context
			if len(newArgs) == 0 {
				showHelpFor = "main"
			} else if len(newArgs) == 1 {
				// Check if this is a namespace command
				if _, isNamespace := namespaceCommands[newArgs[0]]; isNamespace {
					showHelpFor = "namespace:" + newArgs[0]
				} else {
					// Let CLI framework handle regular commands
					newArgs = append(newArgs, arg)
					continue
				}
			} else {
				// For subcommands like "db open --help", let CLI framework handle it
				newArgs = append(newArgs, arg)
				continue
			}
			// Don't add --help to newArgs
			continue
		} else if arg == "help" && i == 0 {
			showHelpFor = "main"
			continue
		}
		newArgs = append(newArgs, arg)
	}

	return newArgs
}

func handleHelpRequest(version string, deprecatedCommands []string) {
	if showHelpFor == "main" {
		// Show main help using the pterm help function
		helpFunc := ptermHelpFunc(version, deprecatedCommands, cli.BasicHelpFunc("trellis"))
		// Create a temporary command map with all available commands
		commands := createCommandMap()
		helpFunc(commands)
		return
	}

	if strings.HasPrefix(showHelpFor, "namespace:") {
		namespaceName := strings.TrimPrefix(showHelpFor, "namespace:")
		showNamespaceHelp(namespaceName)
		return
	}

	if strings.HasPrefix(showHelpFor, "command:") {
		commandName := strings.TrimPrefix(showHelpFor, "command:")
		showCommandHelp(commandName, version)
		return
	}
}

func createCommandMap() map[string]cli.CommandFactory {
	// Return a complete command map for help purposes with proper synopses
	return map[string]cli.CommandFactory{
		// Project commands
		"new": func() (cli.Command, error) { return &mockCommand{synopsis: "Creates a new Trellis project"}, nil },
		"init": func() (cli.Command, error) {
			return &mockCommand{synopsis: "Initializes an existing Trellis project"}, nil
		},

		// Dev commands
		"exec": func() (cli.Command, error) {
			return &mockCommand{synopsis: "Exec runs a command in the Trellis virtualenv"}, nil
		},
		"ssh": func() (cli.Command, error) { return &mockCommand{synopsis: "Connects to host via SSH"}, nil },
		"up": func() (cli.Command, error) {
			return &mockCommand{synopsis: "Starts and provisions the Vagrant environment by running 'vagrant up'"}, nil
		},
		"down": func() (cli.Command, error) {
			return &mockCommand{synopsis: "Stops the Vagrant machine by running 'vagrant halt'"}, nil
		},

		// Deploy commands
		"deploy": func() (cli.Command, error) {
			return &mockCommand{synopsis: "Deploys a site to the specified environment"}, nil
		},
		"provision": func() (cli.Command, error) {
			return &mockCommand{synopsis: "Provisions the specified environment"}, nil
		},
		"rollback": func() (cli.Command, error) {
			return &mockCommand{synopsis: "Rollback the last deploy of the site on the specified environment"}, nil
		},

		// Utils commands
		"alias": func() (cli.Command, error) {
			return &mockCommand{synopsis: "Generate WP CLI aliases for remote environments"}, nil
		},
		"check": func() (cli.Command, error) {
			return &mockCommand{synopsis: "Checks if the required and optional Trellis dependencies are installed"}, nil
		},
		"dotenv": func() (cli.Command, error) { return &mockCommand{synopsis: "Template .env files to local system"}, nil },
		"info": func() (cli.Command, error) {
			return &mockCommand{synopsis: "Displays information about this Trellis project"}, nil
		},
		"logs": func() (cli.Command, error) {
			return &mockCommand{synopsis: "Tails the Nginx log files for an environment"}, nil
		},
		"open": func() (cli.Command, error) {
			return &mockCommand{synopsis: "Opens user-defined URLs (and more) which can act as shortcuts/bookmarks specific to your Trellis projects"}, nil
		},
		"shell-init": func() (cli.Command, error) {
			return &mockCommand{synopsis: "Prints a script which can be eval'd to set up Trellis' virtualenv integration in various shells"}, nil
		},

		// Namespace commands
		"db": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				SynopsisText: "Commands for database management",
			}, nil
		},
		"vm": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				SynopsisText: "Commands for managing development virtual machines",
			}, nil
		},
		"vault": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				SynopsisText: "Commands for Ansible Vault",
			}, nil
		},
		"droplet": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				SynopsisText: "Commands for DigitalOcean Droplets",
			}, nil
		},
		"galaxy": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				SynopsisText: "Commands for Ansible Galaxy",
			}, nil
		},
		"key": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				SynopsisText: "Commands for managing SSH keys",
			}, nil
		},
		"valet": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				SynopsisText: "Commands for Laravel Valet",
			}, nil
		},
		"xdebug-tunnel": func() (cli.Command, error) {
			return &cmd.NamespaceCommand{
				SynopsisText: "Commands for Xdebug tunnel",
			}, nil
		},
	}
}

// mockCommand is a simple command implementation for help display
type mockCommand struct {
	synopsis string
}

func (m *mockCommand) Run([]string) int { return 0 }
func (m *mockCommand) Synopsis() string { return m.synopsis }
func (m *mockCommand) Help() string     { return "" }

func showNamespaceHelp(namespaceName string) {
	subcommands, exists := namespaceCommands[namespaceName]
	if !exists {
		fmt.Printf("Unknown namespace: %s\n", namespaceName)
		return
	}

	namespaceCmd := &cmd.NamespaceCommand{
		HelpText:     fmt.Sprintf("Usage: trellis %s <subcommand> [<args>]", namespaceName),
		SynopsisText: getNamespaceSynopsis(namespaceName),
		Subcommands:  subcommands,
	}

	fmt.Print(namespaceCmd.Help())
}

func getNamespaceSynopsis(namespaceName string) string {
	switch namespaceName {
	case "db":
		return "Commands for database management"
	case "vm":
		return "Commands for managing development virtual machines"
	case "vault":
		return "Commands for Ansible Vault"
	case "droplet":
		return "Commands for DigitalOcean Droplets"
	case "galaxy":
		return "Commands for Ansible Galaxy"
	case "key":
		return "Commands for managing SSH keys"
	case "valet":
		return "Commands for Laravel Valet"
	case "xdebug-tunnel":
		return "Commands for Xdebug tunnel"
	default:
		return "Namespace commands"
	}
}

func showCommandHelp(commandName string, version string) {
	// For subcommands like "db open", we need to create the command and show its help
	parts := strings.Split(commandName, " ")

	if len(parts) == 2 {
		// This is a subcommand like "db open"
		switch commandName {
		case "db open":
			// We can't fully initialize it without dependencies, so show a basic help
			fmt.Printf("\nCommand: trellis %s\n\nFor full help, use: trellis %s --help\n", commandName, commandName)
		default:
			fmt.Printf("\nCommand: trellis %s\n\nFor full help, use: trellis %s --help\n", commandName, commandName)
		}
	} else {
		// Single word command
		fmt.Printf("\nCommand: trellis %s\n\nFor full help, use: trellis %s --help\n", commandName, commandName)
	}
}

func main() {
	// Intercept --help to prevent CLI framework confusion
	// But only when in TTY mode (not in tests)
	args := os.Args[1:]
	if term.IsTerminal(int(os.Stdout.Fd())) {
		args = preprocessArgs(os.Args[1:])

		// Handle help requests immediately, bypassing CLI framework
		if showHelpFor != "" {
			handleHelpRequest(version, deprecatedCommands)
			os.Exit(0)
		}
	}

	c := cli.NewCLI("trellis", version)
	c.Args = args

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
				Subcommands: map[string]string{
					"open": "Open database with GUI applications",
				},
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

	// Use pterm for enhanced help
	c.HelpFunc = ptermHelpFunc(version, deprecatedCommands, cli.BasicHelpFunc("trellis"))

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
