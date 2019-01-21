package trellis

import (
	"errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Site struct {
	Name string
}

type Trellis struct {
	detector     Detector
	Environments map[string][]Site
	Path         string
}

type Config struct {
	WordPressSites map[string]interface{} `yaml:"wordpress_sites"`
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
	os.Chdir(t.Path)

	configPaths, _ := filepath.Glob("group_vars/*/wordpress_sites.yml")

	envs := make([]string, len(configPaths))
	t.Environments = make(map[string][]Site)

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

	for key, _ := range t.Environments {
		names = append(names, key)
	}

	sort.Strings(names)

	return names
}

func (t *Trellis) SiteNamesFromEnvironment(environment string) []string {
	var names []string

	sites := t.Environments[environment]

	for _, site := range sites {
		names = append(names, site.Name)
	}

	sort.Strings(names)

	return names
}

func (t *Trellis) ParseConfig(path string) []Site {
	configYaml, err := ioutil.ReadFile(path)

	if err != nil {
		log.Fatalln(err)
	}

	config := Config{}

	if err = yaml.Unmarshal(configYaml, &config); err != nil {
		log.Fatalln(err)
	}

	sites := make([]Site, len(config.WordPressSites)-1)

	for key, _ := range config.WordPressSites {
		sites = append(sites, Site{Name: key})
	}

	return sites
}
