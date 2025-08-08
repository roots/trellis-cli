package cmd

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/hashicorp/cli"
	"github.com/manifoldco/promptui"
	"github.com/mitchellh/go-homedir"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

const (
	sshKeySecret        = "TRELLIS_DEPLOY_SSH_PRIVATE_KEY"
	sshKnownHostsSecret = "TRELLIS_DEPLOY_SSH_KNOWN_HOSTS"
	deployKeyName       = "Trellis deploy"
)

func NewKeyGenerateCommand(ui cli.Ui, trellis *trellis.Trellis) *KeyGenerateCommand {
	c := &KeyGenerateCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

type KeyGenerateCommand struct {
	UI           cli.Ui
	Trellis      *trellis.Trellis
	flags        *flag.FlagSet
	keyName      string
	knownHosts   string
	noGithub     bool
	noProvision  bool
	path         string
	provisionEnv string
	repo         string
}

func (c *KeyGenerateCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.noGithub, "no-github", false, "Skips creating a GitHub secret and deploy key")
	c.flags.BoolVar(&c.noProvision, "no-provision", false, "Skips provisioning the environment after key is generated")
	c.flags.StringVar(&c.keyName, "key-name", "", "Name of SSH key (Default: trellis_<site_name>_ed25519).")
	c.flags.StringVar(&c.knownHosts, "known-hosts", "", "Comma-separated list of SSH known hosts (optional)")
	c.flags.StringVar(&c.path, "path", "", "Path of private key (Default: $HOME/.ssh)")
	c.flags.StringVar(&c.provisionEnv, "provision", "", "Environment to provision after key is generated")
	c.flags.StringVar(&c.repo, "repo", "", "Repository to add the GitHub secret and deploy key to. Format: OWNER/REPO")
}

func (c *KeyGenerateCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	if !c.noGithub {
		_, err := exec.LookPath("gh")
		if err != nil {
			c.UI.Error("Error: GitHub CLI not found")
			c.UI.Error("gh command must be available to interact with GitHub")
			c.UI.Error("See https://cli.github.com")
			c.UI.Error("")
			c.UI.Error("To skip GitHub integration, re-run this command with the --no-github option.")
			return 1
		}

		_, err = command.Cmd("gh", []string{"auth", "status"}).Output()
		if err != nil {
			c.UI.Error("Error: GitHub CLI is not authenticated.")
			c.UI.Error("Run `gh auth login` first.")
			c.UI.Error("")
			c.UI.Error("To skip GitHub integration, re-run this command with the --no-github option.")
			return 1
		}
	}

	if c.keyName == "" {
		siteName, _, siteErr := c.Trellis.MainSiteFromEnvironment("development")
		if siteErr != nil {
			c.UI.Error(siteErr.Error())
			return 1
		}

		c.keyName = fmt.Sprintf("trellis_%s", strings.ReplaceAll(siteName, ".", "_"))
	}

	c.keyName = fmt.Sprintf("%s_ed25519", c.keyName)
	publicKeyName := fmt.Sprintf("%s.pub", c.keyName)

	if c.path == "" {
		homePath, _ := homedir.Dir()
		path := filepath.Join(homePath, ".ssh")
		c.path = path
	}

	keyPath := filepath.Join(c.path, c.keyName)
	publicKeyPath := filepath.Join(c.path, publicKeyName)
	trellisPublicKeysPath := filepath.Join(c.Trellis.Path, "public_keys")
	trellisPublicKeyPath := filepath.Join(trellisPublicKeysPath, publicKeyName)
	_ = os.Mkdir(trellisPublicKeysPath, os.ModePerm)

	keyExists, _ := os.Stat(keyPath)
	publicKeyExists, _ := os.Stat(trellisPublicKeyPath)

	if keyExists != nil || publicKeyExists != nil {
		c.UI.Error("Error: keys already exist. Delete them first if you want to re-generate a new key.")
		c.UI.Error(fmt.Sprintf("Private key: %s", keyPath))
		c.UI.Error(fmt.Sprintf("Public key: %s", trellisPublicKeyPath))
		return 1
	}

	if err := generateKey(deployKeyName, keyPath); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Info(fmt.Sprintf("%s Generated SSH key [%s]", color.GreenString("[✓]"), keyPath))

	err := os.Rename(publicKeyPath, trellisPublicKeyPath)

	if err != nil {
		c.UI.Error("Error: could not move public key")
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Info(fmt.Sprintf("%s Moved public key [%s]", color.GreenString("[✓]"), trellisPublicKeyPath))

	if c.noGithub {
		// the rest of the command is all GitHub integration
		return 0
	}

	if err := setPrivateKeySecret(keyPath, c.repo); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Info(fmt.Sprintf("%s GitHub private key secret set [%s]", color.GreenString("[✓]"), sshKeySecret))

	if err := setDeployKey(deployKeyName, trellisPublicKeyPath, c.repo); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Info(fmt.Sprintf("%s GitHub deploy key added [%s]", color.GreenString("[✓]"), deployKeyName))

	if c.knownHosts == "" {
		hosts, err := getAnsibleHosts()
		if err != nil {
			c.UI.Warn("Warning: could not get Ansible hosts as defaults for known hosts")
		}

		c.UI.Info("\nBefore the new SSH key can be used, GitHub's action runner also needs one or more SSH known hosts.")
		c.UI.Info(fmt.Sprintf("The following hosts were automatically detected: %s", strings.Join(hosts, ", ")))
		c.UI.Info("If that list of hosts is correct, you can accept the default. Or provide a comma-separated list of hosts instead.\n")

		prompt := promptui.Prompt{
			Label:   "SSH known hosts (comma-separated list)",
			Default: strings.Join(hosts, ", "),
		}

		hostsInput, err := prompt.Run()
		c.knownHosts = hostsInput

		if err != nil {
			c.UI.Error("Aborting: no known hosts provided")
			return 1
		}
	}

	if err := setSshKnownHostsSecret(sshKnownHostsSecret, c.knownHosts, c.repo); err != nil {
		c.UI.Error("Error: could not set SSH known hosts.")
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Info(fmt.Sprintf("%s GitHub known hosts secret set [%s]", color.GreenString("[✓]"), sshKnownHostsSecret))

	if c.noProvision {
		// the rest of the command is environment provisioning
		return 0
	}

	if c.provisionEnv == "" {
		c.UI.Info("\nThe public key will not be usable until it's added to your server.")
		prompt := promptui.Prompt{
			Label:     "Provision now and apply the new public key",
			IsConfirm: true,
		}
		_, err = prompt.Run()

		if err != nil {
			return 0
		}

		environments := c.Trellis.EnvironmentNames()

		envPrompt := promptui.Select{
			Label: "Select environment to provision",
			Items: environments,
			Size:  len(environments),
		}

		i, _, err := envPrompt.Run()

		if err != nil {
			c.UI.Error("Provision aborted")
			return 0
		}
		c.provisionEnv = environments[i]
	}

	provisionCmd := NewProvisionCommand(c.UI, c.Trellis)
	return provisionCmd.Run([]string{"--tags", "users", c.provisionEnv})
}

func (c *KeyGenerateCommand) Synopsis() string {
	return "Generates an SSH key for Trellis deploys"
}

func (c *KeyGenerateCommand) Help() string {
	helpText := `
Usage: trellis key generate

Generates an SSH key (using Ed25519 algorithm) for Trellis deploys with GitHub integration.

* public key is created in 'trellis/public_keys' and added as a GitHub Deploy Key
* private key is created in '$HOME/.ssh' and added as a GitHub Secret (so its accessible within GitHub Actions)

This command relies on the GitHub CLI being installed and authenticated properly.
See https://cli.github.com for more details and installation instructions.

The GitHub repo used is detected automatically by the GitHub CLI.
This command will fail if there's no GitHub repo or you don't have access to it.

To skip the GitHub specific parts, use the --no-github option:

  $ trellis key generate --no-github

Specify a custom key name:

  $ trellis key generate --name "my_key"

Generate private key in a specific path:

  $ trellis key generate --path ~/my_keys

Specify a repository to add the GitHub secret and deploy key to:

  $ trellis key generate --repo MyOrg/MyBedrockRepo

To skip provisioning, use the --no-provision option:
  
  $ trellis key generate --no-provision

Options:
      --known-hosts    Comma-separated list of SSH known hosts (optional)
      --name           Name of SSH key (Default: trellis_<site_name>_ed25519)
      --no-github      Skips creating a GitHub secret and deploy key
      --path           Path for private key (Default: $HOME/.ssh)
      --[no-]provision Name of environment to provision after key is generated
      --repo           Repository to add the GitHub secret and deploy key to (Format: OWNER/REPO)
  -h, --help           show this help
`

	return strings.TrimSpace(helpText)
}

func (c *KeyGenerateCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *KeyGenerateCommand) AutocompleteFlags() complete.Flags {
	var environmentNames []string

	if err := c.Trellis.LoadProject(); err == nil {
		environmentNames = c.Trellis.EnvironmentNames()
	}

	return complete.Flags{
		"--known-hosts": complete.PredictNothing,
		"--name":        complete.PredictNothing,
		"--no-github":   complete.PredictNothing,
		"--path":        complete.PredictDirs(""),
		"--provision":   complete.PredictSet(environmentNames...),
		"--repo":        complete.PredictNothing,
	}
}

func githubCLI(args ...string) error {
	ghCmd := command.Cmd("gh", args)
	ghCmd.Stdout = io.Discard
	ghCmd.Stderr = os.Stderr

	return ghCmd.Run()
}

func generateKey(name string, path string) error {
	keygenArgs := []string{"-t", "ed25519", "-C", name, "-f", path, "-P", ""}
	sshKeygen := command.Cmd("ssh-keygen", keygenArgs)
	sshKeygen.Stdout = io.Discard
	sshKeygen.Stderr = os.Stderr
	err := sshKeygen.Run()

	if err != nil {
		return fmt.Errorf("Error: could not generate SSH key\n%v", err)
	}

	return nil
}

func getAnsibleHosts() (hosts []string, err error) {
	args := []string{"all", "--list-hosts", "--limit", "!development"}
	hostsOutput, err := command.Cmd("ansible", args).Output()

	if err != nil {
		return nil, err
	}

	hosts = parseAnsibleHosts(string(hostsOutput))

	if len(hosts) == 0 {
		return nil, errors.New("No hosts found by Ansible. This is either a bug in trellis-cli or your host files are invalid.")
	}

	return hosts, nil
}

/*

Parses the output of `ansible all --list-hosts` into a slice of host strings

Example input:
  hosts (3):
    192.168.56.5
    192.168.56.10
    your_server_hostname
*/

func parseAnsibleHosts(output string) (hosts []string) {
	lines := strings.Split(string(output), "\n")

	for _, host := range lines {
		host = strings.TrimSpace(host)

		if strings.HasPrefix(host, "hosts (") || host == "" {
			continue
		}

		// remove default placeholder since it will cause  an error
		// this isn't ideal, but it will do
		if host == "your_server_hostname" {
			continue
		}

		hosts = append(hosts, host)
	}

	return hosts
}

func keyscanHosts(hosts []string) (knownHosts []string) {
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		output, err := command.Cmd("ssh-keyscan", []string{"-t", "ed25519", "-H", "-T", "1", host}).Output()

		if err == nil {
			knownHosts = append(knownHosts, string(output))
		}
	}

	return knownHosts
}

func setDeployKey(name string, path string, repo string) error {
	publicKeyContent, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("Error: could not read SSH public key file\n%v", err)
	}

	publicKeyContent = bytes.TrimSuffix(publicKeyContent, []byte("\n"))
	title := fmt.Sprintf("title=%s", name)
	key := fmt.Sprintf("key=%s", string(publicKeyContent))
	endpoint := "repos/{owner}/{repo}/keys"
	if repo != "" {
		endpoint = strings.Replace(endpoint, "{owner}/{repo}", repo, 1)
	}

	err = githubCLI("api", endpoint, "-f", title, "-f", key, "-f", "read_only=true")
	if err != nil {
		return fmt.Errorf("Error: could not create GitHub deploy key\n%v", err)
	}

	return nil
}

func setPrivateKeySecret(path string, repo string) error {
	privateKeyContent, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("Error: could not read SSH private key file\n%v", err)
	}

	ghCLIArgs := []string{"secret", "set", sshKeySecret, "--body", string(privateKeyContent)}
	if repo != "" {
		ghCLIArgs = append(ghCLIArgs, "--repo", repo)
	}

	err = githubCLI(ghCLIArgs...)
	if err != nil {
		return fmt.Errorf("could not set GitHub secret\n%v", err)
	}

	return nil
}

func setSshKnownHostsSecret(sshKnownHostsSecret string, knownHosts string, repo string) error {
	sshKnownHosts := keyscanHosts(strings.Split(knownHosts, ","))
	if len(sshKnownHosts) == 0 {
		return fmt.Errorf("ssh-keyscan command failed for all hosts: %s", sshKnownHosts)
	}

	ghCLIArgs := []string{"secret", "set", sshKnownHostsSecret, "--body", strings.Join(sshKnownHosts, "\n")}
	if repo != "" {
		ghCLIArgs = append(ghCLIArgs, "--repo", repo)
	}

	err := githubCLI(ghCLIArgs...)
	if err != nil {
		return fmt.Errorf("Error: could not set GitHub secret\n%v", err)
	}

	return nil
}
