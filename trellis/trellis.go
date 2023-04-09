package trellis

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/app_paths"
	"github.com/roots/trellis-cli/cli_config"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v2"
)

const (
	ConfigDir   = ".trellis"
	GlobPattern = "group_vars/*/wordpress_sites.yml"
)

const cliConfigFile = "cli.yml"

type Options struct {
	Detector  Detector
	ConfigDir string
}

type TrellisOption func(*Trellis)

var DefaultCliConfig = cli_config.Config{
	AllowDevelopmentDeploys: false,
	AskVaultPass:            false,
	CheckForUpdates:         true,
	LoadPlugins:             true,
	Open:                    make(map[string]string),
	VirtualenvIntegration:   true,
	Vm: cli_config.VmConfig{
		Manager:       "auto",
		HostsResolver: "hosts_file",
		Ubuntu:        "22.04",
	},
}

type Trellis struct {
	CliConfig       cli_config.Config
	ConfigDir       string
	Detector        Detector
	Environments    map[string]*Config
	Path            string
	Virtualenv      *Virtualenv
	VenvInitialized bool
	venvWarned      bool
}

func NewTrellis(opts ...TrellisOption) *Trellis {
	const (
		defaultConfigDir       = ConfigDir
		defaultVenvInitialized = false
		defaultVenvWarned      = false
	)

	t := &Trellis{
		CliConfig:       cli_config.NewConfig(DefaultCliConfig),
		ConfigDir:       defaultConfigDir,
		Detector:        &ProjectDetector{},
		VenvInitialized: defaultVenvInitialized,
		venvWarned:      defaultVenvWarned,
	}

	for _, opt := range opts {
		opt(t)
	}

	return t
}

func WithOptions(options *Options) TrellisOption {
	return func(t *Trellis) {
		t.Detector = options.Detector
		t.ConfigDir = options.ConfigDir
	}
}

func NewMockTrellis(projectDetected bool) *Trellis {
	return NewTrellis(
		WithOptions(
			&Options{Detector: &MockProjectDetector{detected: projectDetected}},
		),
	)
}

/*
Detect if a path is a Trellis project or not
This will traverse up the directory tree until it finds a valid project,
or stop at the root and give up.
*/
func (t *Trellis) Detect(path string) (projectPath string, ok bool) {
	return t.Detector.Detect(path)
}

func (t *Trellis) ConfigPath() string {
	return filepath.Join(t.Path, t.ConfigDir)
}

func (t *Trellis) CreateConfigDir() error {
	if err := os.Mkdir(t.ConfigPath(), 0755); err != nil && !os.IsExist(err) {
		return err
	}

	return nil
}

func (t *Trellis) CheckVirtualenv(ui cli.Ui) {
	if t.CliConfig.VirtualenvIntegration && !t.venvWarned && !t.VenvInitialized {
		ui.Warn(`
WARNING: This project has not been initialized with trellis-cli and may not work as expected.

You may see this warning if you are using trellis-cli on an existing project (previously created without the CLI).
To ensure you have the required dependencies, initialize the project with the following command:

  $ trellis init

If you want to manage dependencies yourself manually, you can ignore this warning.
To disable this automated check, set the 'virtualenv_integration' configuration setting to 'false'.

  `)
	}

	t.venvWarned = true
}

/*
Activates a Trellis project's virtualenv without loading the config files.
This is optimized to be a lighter weight version of LoadProject more suitable
for the shell hook.
*/
func (t *Trellis) ActivateProject() bool {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	path, ok := t.Detect(wd)

	if !ok {
		return false
	}

	t.Path = path
	t.Virtualenv = NewVirtualenv(t.ConfigPath())

	if !t.Virtualenv.Initialized() {
		return false
	}

	os.Chdir(t.Path)

	return true
}

/*
Loads a Trellis project.
If a project is detected, the wordpress_sites config files are parsed and
the directory is changed to the project path.
*/
func (t *Trellis) LoadProject() error {
	if t.Path != "" {
		os.Chdir(t.Path)
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Fatal error getting current working directory: %v", err)
	}

	path, ok := t.Detect(wd)

	if !ok {
		return errors.New("No Trellis project detected in the current directory or any of its parent directories.")
	}

	if err = t.LoadProjectCliConfig(); err != nil {
		return err
	}

	t.Path = path
	t.Virtualenv = NewVirtualenv(t.ConfigPath())

	os.Chdir(t.Path)

	if t.CliConfig.VirtualenvIntegration {
		if t.Virtualenv.Initialized() {
			t.VenvInitialized = true
			t.Virtualenv.Activate()
		}
	}

	configPaths, _ := filepath.Glob("group_vars/*/wordpress_sites.yml")

	envs := make([]string, len(configPaths))
	t.Environments = make(map[string]*Config, len(configPaths)-1)

	for i, p := range configPaths {
		parts := strings.Split(p, string(os.PathSeparator))
		envName := parts[1]
		envs[i] = envName

		t.Environments[envName] = t.ParseConfig(p)
	}

	return nil
}

func (t *Trellis) EnvironmentNames() []string {
	var names []string

	for key := range t.Environments {
		names = append(names, key)
	}

	sort.Strings(names)

	return names
}

func (t *Trellis) ValidateEnvironment(name string) (err error) {
	_, ok := t.Environments[name]
	if ok {
		return nil
	}

	return fmt.Errorf("Error: %s is not a valid environment, valid options are %s", name, t.EnvironmentNames())
}

func (t *Trellis) SiteNamesFromEnvironment(environment string) []string {
	var names []string

	config := t.Environments[environment]

	for name := range config.WordPressSites {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

func (t *Trellis) FindSiteNameFromEnvironment(environment string, siteNameArg string) (string, error) {
	if siteNameArg == "" {
		return t.getDefaultSiteNameFromEnvironment(environment)
	}

	siteNames := t.SiteNamesFromEnvironment(environment)
	for _, siteName := range siteNames {
		if siteName == siteNameArg {
			return siteName, nil
		}
	}

	return "", fmt.Errorf("Error: %s is not a valid site. Valid options are %s", siteNameArg, siteNames)
}

func (t *Trellis) MainSiteFromEnvironment(environment string) (string, *Site, error) {
	sites := t.SiteNamesFromEnvironment(environment)

	if len(sites) == 0 {
		return "", nil, fmt.Errorf("Error: No sites found in %s environment", environment)
	}

	name := sites[0]

	return name, t.Environments[environment].WordPressSites[name], nil
}

func (t *Trellis) getDefaultSiteNameFromEnvironment(environment string) (siteName string, err error) {
	sites := t.SiteNamesFromEnvironment(environment)

	siteCount := len(sites)
	switch {
	case siteCount == 0:
		return "", fmt.Errorf("Error: No sites found in %s", environment)
	case siteCount > 1:
		return "", fmt.Errorf("Error: Multiple sites found in %s. Please specific a site. Valid options are %s", environment, sites)
	}

	return sites[0], nil
}

func (t *Trellis) LoadCliConfig() error {
	path := app_paths.ConfigPath(cliConfigFile)

	if err := t.CliConfig.LoadFile(path); err != nil {
		return fmt.Errorf("Error loading CLI config %s\n\n%v", path, err)
	}

	if err := t.CliConfig.LoadEnv("TRELLIS_"); err != nil {
		return fmt.Errorf("Error loading CLI config\n\n%v", err)
	}

	return nil
}

func (t *Trellis) LoadProjectCliConfig() error {
	path := filepath.Join(t.ConfigPath(), cliConfigFile)

	if err := t.CliConfig.LoadFile(path); err != nil {
		return fmt.Errorf("Error loading CLI config %s\n%v", path, err)
	}

	t.CliConfig.LoadEnv("TRELLIS_")

	if t.CliConfig.AskVaultPass {
		// https://docs.ansible.com/ansible/latest/reference_appendices/config.html#default-ask-vault-pass
		os.Setenv("ANSIBLE_ASK_VAULT_PASS", "true")
	}

	return nil
}

func (t *Trellis) SiteFromEnvironmentAndName(environment string, name string) *Site {
	return t.Environments[environment].WordPressSites[name]
}

func (t *Trellis) UpdateAnsibleConfig(section string, key string, value string) error {
	ansibleCfg := filepath.Join(t.Path, "ansible.cfg")
	cfg, err := ini.Load(ansibleCfg)

	if err != nil {
		return err
	}

	cfg.Section(section).Key(key).SetValue(value)
	if err := cfg.SaveTo(ansibleCfg); err != nil {
		return err
	}

	return nil
}

func (t *Trellis) WriteYamlFile(s interface{}, path string, header string) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		log.Fatal(err)
	}

	path = filepath.Join(t.Path, path)
	data = append([]byte(header), data...)

	if err := os.WriteFile(path, data, 0666); err != nil {
		log.Fatal(err)
	}

	return nil
}
