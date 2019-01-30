package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/mholt/archiver"
	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

type NewCommand struct {
	UI      cli.Ui
	flags   *flag.FlagSet
	force   bool
	trellis *trellis.Trellis
}

type Release struct {
	Version string `json:"tag_name"`
	ZipUrl  string `json:"zipball_url"`
}

func NewNewCommand(ui cli.Ui, trellis *trellis.Trellis) *NewCommand {
	c := &NewCommand{UI: ui, trellis: trellis}
	c.init()
	return c
}

func (c *NewCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.force, "force", false, "Forces the creation of the project even if the target path is not empty")
}

func (c *NewCommand) Run(args []string) int {
	var path string

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	switch len(args) {
	case 0:
		c.UI.Error("Missing PATH argument\n")
		c.UI.Output(c.Help())
		return 1
	case 1:
		path = args[0]
	default:
		c.UI.Error(fmt.Sprintf("Error: too many arguments (expected 1, got %d)\n", len(args)))
		c.UI.Output(c.Help())
		return 1
	}

	path, _ = filepath.Abs(path)
	_, err := os.Stat(path)

	c.UI.Info(fmt.Sprintf("Creating new Trellis project in %s\n", path))

	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(path, os.ModePerm); err != nil {
				log.Fatal(err)
			}
		} else {
			log.Fatal(err)
		}
	}

	if !c.force {
		isPathEmpty, _ := isDirEmpty(path)

		if !isPathEmpty {
			c.UI.Error(fmt.Sprintf("%s path is not empty. Use --force option to skip this check.", path))
			return 1
		}
	}

	name, err := askProjectName(c.UI)
	if err != nil {
		return 1
	}

	host, err := askDomain(c.UI, name)
	if err != nil {
		return 1
	}

	fmt.Println("\nFetching latest versions of Trellis and Bedrock...")

	trellisPath := filepath.Join(path, "trellis")
	trellisVersion := downloadLatestRelease("roots/trellis", path, trellisPath)
	bedrockVersion := downloadLatestRelease("roots/bedrock", path, filepath.Join(path, "site"))

	if addTrellisFile(trellisPath) != nil {
		log.Fatal("Error writing .trellis.yml file")
	}

	os.Chdir(path)

	if err := c.trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	// Update default configs
	for env, config := range c.trellis.Environments {
		c.trellis.UpdateDefaultConfig(config, name, host, env)
		c.trellis.WriteConfigYaml(config, filepath.Join("group_vars", env, "wordpress_sites.yml"))

		stringGenerator := trellis.RandomStringGenerator{Length: 64}
		vault := c.trellis.GenerateVaultConfig(name, env, &stringGenerator)
		c.trellis.WriteVaultYaml(vault, filepath.Join("group_vars", env, "vault.yml"))
	}

	if err := c.trellis.GenerateVaultPassFile(".vault_pass"); err != nil {
		c.UI.Error("Error writing .vault_pass file:")
		c.UI.Error(err.Error())
	}

	if err := c.trellis.UpdateAnsibleConfig("defaults", "vault_password_file", ".vault_pass"); err != nil {
		c.UI.Error("Error adding vault_password_file setting to ansible.cfg:")
		c.UI.Error(err.Error())
	}

	fmt.Printf("\n%s project created with versions:\n", color.GreenString(name))
	fmt.Printf("  Trellis v%s\n", trellisVersion)
	fmt.Printf("  Bedrock v%s\n", bedrockVersion)

	return 0
}

func (c *NewCommand) Synopsis() string {
	return "Creates a new Trellis project"
}

func (c *NewCommand) Help() string {
	helpText := `
Usage: trellis new [PATH]

Creates a new Trellis project in the path specified using the latest versions of Trellis and Bedrock.

This uses our recommended project structure detailed at
https://roots.io/trellis/docs/installing-trellis/#create-a-project

Create a new project in the current directory:

  $ trellis new .

Create a new project in the target path:

  $ trellis new ~/dev/example.com

Force create a new project in a non-empty target path:

  $ trellis new --force ~/dev/example.com

Arguments:
  PATH  Path to create new project in

Options:
  --force     (default: false) Forces the creation of the project even if the target path is not empty
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}

func addTrellisFile(path string) error {
	path = filepath.Join(path, ".trellis.yml")
	return ioutil.WriteFile(path, []byte{}, 0666)
}

func askProjectName(ui cli.Ui) (name string, err error) {
	name, err = ui.Ask(fmt.Sprintf("%s:", color.MagentaString("Project name")))

	if err != nil {
		return "", err
	}

	if len(name) == 0 {
		return askProjectName(ui)
	}

	return name, nil
}

func askDomain(ui cli.Ui, name string) (host string, err error) {
	host, err = ui.Ask(fmt.Sprintf("%s [%s]:", color.MagentaString("Site domain"), color.GreenString(name)))

	if err != nil {
		return "", err
	}

	if len(host) == 0 {
		return name, nil
	}

	return host, nil
}

func downloadLatestRelease(repo string, path string, dest string) string {
	release := fetchLatestRelease(repo)

	os.Chdir(path)
	archivePath := fmt.Sprintf("%s.zip", release.Version)

	err := downloadFile(archivePath, release.ZipUrl)
	if err != nil {
		log.Fatal(err)
	}

	if err := archiver.Unarchive(archivePath, path); err != nil {
		log.Fatal(err)
	}

	dirs, _ := filepath.Glob("roots-*")

	if len(dirs) == 0 {
		log.Fatalln("Error: extracted release zip did not contain the expected directory")
	}

	for _, dir := range dirs {
		err := os.Rename(dir, dest)

		if err != nil {
			log.Fatal(err)
		}
	}

	err = os.Remove(archivePath)

	if err != nil {
		log.Fatal(err)
	}

	return release.Version
}

func fetchLatestRelease(repo string) Release {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := http.Get(url)

	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	release := Release{}

	if err = json.Unmarshal(body, &release); err != nil {
		log.Fatal(err)
	}

	return release
}

func isDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)

	if err != nil {
		return false, err
	}

	defer f.Close()

	if _, err = f.Readdirnames(1); err == io.EOF {
		return true, nil
	}

	return false, err
}

func downloadFile(filepath string, url string) error {
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
