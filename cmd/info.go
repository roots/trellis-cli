package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/pkg/lima"
	"github.com/roots/trellis-cli/trellis"
)

type InfoCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	json    bool
}

func NewInfoCommand(ui cli.Ui, trellis *trellis.Trellis) *InfoCommand {
	c := &InfoCommand{UI: ui, Trellis: trellis}
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.json, "json", false, "Output as JSON")
	return c
}

type infoData struct {
	Path           string                `json:"path"`
	TrellisVersion string                `json:"trellis_version,omitempty"`
	Virtualenv     string                `json:"virtualenv"`
	VM             string                `json:"vm"`
	Sites          map[string][]siteInfo `json:"sites"`
}

type siteInfo struct {
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Redirects []string `json:"redirects,omitempty"`
	LocalPath string   `json:"local_path"`
	SSL       bool     `json:"ssl"`
	Cache     bool     `json:"cache"`
}

func (c *InfoCommand) Run(args []string) int {
	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	data := c.collectInfo()

	if c.json {
		jsonBytes, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			c.UI.Error(fmt.Sprintf("Error encoding JSON: %s", err))
			return 1
		}
		c.UI.Output(string(jsonBytes))
		return 0
	}

	c.printInfo(data)
	return 0
}

func (c *InfoCommand) collectInfo() infoData {
	venvStatus := "inactive"
	if c.Trellis.Virtualenv != nil && c.Trellis.Virtualenv.Active() {
		venvStatus = "active"
	} else if c.Trellis.Virtualenv != nil && c.Trellis.Virtualenv.Initialized() {
		venvStatus = "initialized"
	}

	vmStatus := c.vmStatus()

	sites := make(map[string][]siteInfo)
	for _, env := range c.Trellis.EnvironmentNames() {
		config := c.Trellis.Environments[env]
		var envSites []siteInfo

		for name, site := range config.WordPressSites {
			si := siteInfo{
				Name:      name,
				URL:       site.MainUrl(),
				LocalPath: site.LocalPath,
				SSL:       site.SslEnabled(),
				Cache:     site.Cache["enabled"] == true,
			}

			if len(site.SiteHosts) > 0 {
				si.Redirects = site.SiteHosts[0].Redirects
			}

			envSites = append(envSites, si)
		}

		sites[env] = envSites
	}

	trellisVersion := ""
	versionFile := filepath.Join(c.Trellis.Path, "trellis", "VERSION")
	if data, err := os.ReadFile(versionFile); err == nil {
		trellisVersion = strings.TrimSpace(string(data))
	}

	return infoData{
		Path:           c.Trellis.Path,
		TrellisVersion: trellisVersion,
		Virtualenv:     venvStatus,
		VM:             vmStatus,
		Sites:          sites,
	}
}

func (c *InfoCommand) vmStatus() string {
	vmType := c.Trellis.VmManagerType()
	if vmType == "" {
		return "none"
	}

	if vmType != "lima" {
		return vmType
	}

	manager, err := lima.NewManager(c.Trellis, c.UI)
	if err != nil {
		return vmType
	}

	instanceName, err := c.Trellis.GetVmInstanceName()
	if err != nil {
		return vmType
	}

	instance, ok := manager.GetInstance(instanceName)
	if !ok {
		return fmt.Sprintf("%s (not created)", vmType)
	}

	if instance.Running() {
		return fmt.Sprintf("%s (running)", vmType)
	}

	return fmt.Sprintf("%s (stopped)", vmType)
}

func (c *InfoCommand) printInfo(data infoData) {
	bold := color.New(color.Bold).SprintFunc()

	c.UI.Output(fmt.Sprintf("%s %s", bold("Project:"), data.Path))

	if data.TrellisVersion != "" {
		c.UI.Output(fmt.Sprintf("%s %s", bold("Trellis:"), data.TrellisVersion))
	}

	c.UI.Output(fmt.Sprintf("%s %s", bold("Virtualenv:"), data.Virtualenv))
	c.UI.Output(fmt.Sprintf("%s %s", bold("VM:"), data.VM))
	c.UI.Output("")

	for _, env := range c.Trellis.EnvironmentNames() {
		sites := data.Sites[env]

		c.UI.Output(bold(env))

		for _, site := range sites {
			c.UI.Output(fmt.Sprintf("  %s", site.Name))
			c.UI.Output(fmt.Sprintf("    URL:        %s", site.URL))

			if len(site.Redirects) > 0 {
				c.UI.Output(fmt.Sprintf("    Redirects:  %s", strings.Join(site.Redirects, ", ")))
			}

			c.UI.Output(fmt.Sprintf("    Local Path: %s", site.LocalPath))
			c.UI.Output(fmt.Sprintf("    SSL:        %t", site.SSL))
			c.UI.Output(fmt.Sprintf("    Cache:      %t", site.Cache))
		}

		c.UI.Output("")
	}
}

func (c *InfoCommand) Synopsis() string {
	return "Displays information about this Trellis project"
}

func (c *InfoCommand) Help() string {
	helpText := `
Usage: trellis info [options]

Displays information about this Trellis project

Options:
      --json  Output as JSON
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}
