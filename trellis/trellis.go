package trellis

import (
	"errors"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const ConfigDir string = ".trellis"

type Trellis struct {
	detector     Detector
	Environments map[string]*Config
	ConfigPath   string
	Path         string
	Virtualenv   *Virtualenv
}

func NewTrellis(d Detector) *Trellis {
	return &Trellis{detector: d}
}

/*
Detect if a path is a Trellis project or not
This will traverse up the directory tree until it finds a valid project,
or stop at the root and give up.
*/
func (t *Trellis) Detect(path string) (projectPath string, ok bool) {
	return t.detector.Detect(path)
}

func (t *Trellis) CreateConfigDir() error {
	_, err := os.Stat(ConfigDir)

	if os.IsExist(err) {
		return nil
	}

	if os.IsNotExist(err) {
		return os.Mkdir(ConfigDir, 0755)
	}

	return nil
}

/*
Loads a Trellis project.
If a project is detected, the wordpress_sites config files are parsed and
the directory is changed to the project path.
*/
func (t *Trellis) LoadProject() error {
	wd, err := os.Getwd()

	if err != nil {
		log.Fatal(err)
	}

	path, ok := t.Detect(wd)

	if !ok {
		return errors.New("No Trellis project detected in the current directory or any of its parent directories.")
	}

	t.Path = path
	t.ConfigPath = filepath.Join(path, ConfigDir)
	t.Virtualenv = NewVirtualenv(t.ConfigPath)

	os.Chdir(t.Path)

	configPaths, _ := filepath.Glob("group_vars/*/wordpress_sites.yml")

	envs := make([]string, len(configPaths))
	t.Environments = make(map[string]*Config, len(configPaths)-1)

	for i, p := range configPaths {
		parts := strings.Split(p, string(os.PathSeparator))
		envName := parts[1]
		envs[i] = envName

		t.Environments[envName] = t.ParseConfig(p)
	}

	if os.Getenv("TRELLIS_VENV") != "false" {
		if t.Virtualenv.Initialized() {
			t.Virtualenv.Activate()
		}
	}

	return nil
}

func (t *Trellis) EnvironmentNames() []string {
	var names []string

	for key, _ := range t.Environments {
		names = append(names, key)
	}

	sort.Strings(names)

	return names
}

func (t *Trellis) SiteNamesFromEnvironment(environment string) []string {
	var names []string

	config := t.Environments[environment]

	for name, _ := range config.WordPressSites {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
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

	if err := ioutil.WriteFile(path, data, 0666); err != nil {
		log.Fatal(err)
	}

	return nil
}
