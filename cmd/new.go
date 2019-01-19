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

	"github.com/mholt/archiver"
	"github.com/mitchellh/cli"
)

type NewCommand struct {
	UI    cli.Ui
	flags *flag.FlagSet
	force bool
}
type Release struct {
	Version string `json:"tag_name"`
	ZipUrl  string `json:"zipball_url"`
}

func NewNewCommand(ui cli.Ui) *NewCommand {
	c := &NewCommand{UI: ui}
	c.init()
	return c
}

func (c *NewCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.force, "force", false, "Forces the creation of the project even if the target path is not empty")
}

func (c *NewCommand) Run(args []string) int {
	var name string
	var path string

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	switch len(args) {
	case 0:
		c.UI.Error("Missing NAME argument\n")
		c.UI.Output(c.Help())
		return 1
	case 1:
		name = args[0]
	case 2:
		name = args[0]
		path = args[1]
	default:
		c.UI.Error(fmt.Sprintf("Error: too many arguments (expected 2, got %d)\n", len(args)))
		c.UI.Output(c.Help())
		return 1
	}

	path, _ = filepath.Abs(path)
	_, err := os.Stat(path)

	fmt.Println("Creating new Trellis project in", path)

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

	fmt.Println("Fetching latest versions of Trellis and Bedrock...")
	trellisVersion := downloadLatestRelease("roots/trellis", path, filepath.Join(path, "trellis"))
	bedrockVersion := downloadLatestRelease("roots/bedrock", path, filepath.Join(path, "bedrock"))

	fmt.Printf("\n%s project created with versions:\n", name)
	fmt.Printf("  Trellis v%s\n", trellisVersion)
	fmt.Printf("  Bedrock v%s\n", bedrockVersion)

	return 0
}

func (c *NewCommand) Synopsis() string {
	return "Creates a new Trellis project"
}

func (c *NewCommand) Help() string {
	helpText := `
Usage: trellis new NAME [PATH]

Creates a new Trellis project in the path specified (defaults to current directory)
using the latest versions of Trellis and Bedrock.

This uses our recommended project structure detailed at
https://roots.io/trellis/docs/installing-trellis/#create-a-project

Create a new project in the current directory:

  $ trellis new example.com

Create a new project in the target path:

  $ trellis new example.com ~/dev/example.com

Force create a new project in a non-empty target path:

  $ trellis new --force example.com ~/dev/example.com

Arguments:
  NAME  Name of new Trellis project (ie: example.com)
  PATH  Path to create new project in
        (default: .)

Options:
  --force     (default: false) Forces the creation of the project even if the target path is not empty
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
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
