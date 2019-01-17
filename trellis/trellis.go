package trellis

import (
	"github.com/mitchellh/cli"
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
	Environments map[string][]Site
}

type Config struct {
	WordPressSites map[string]interface{} `yaml:"wordpress_sites"`
}

func Init() *Trellis {
	trellis := &Trellis{}
	trellis.Init()
	return trellis
}

func (t *Trellis) Detect() []string {
	wd, err := os.Getwd()

	if err != nil {
		log.Fatal(err)
	}

	paths, _ := filepath.Glob("group_vars/*/wordpress_sites.yml")

	if len(paths) == 0 {
		parent := filepath.Dir(wd)

		if len(parent) == 1 {
			if parent == "." || os.IsPathSeparator(parent[0]) {
				return []string{}
			}
		}

		if err := os.Chdir(parent); err != nil {
			log.Fatal(err)
		}

		return t.Detect()
	}

	return paths
}

func (t *Trellis) Init() {
	paths := t.Detect()

	envs := make([]string, len(paths))
	t.Environments = make(map[string][]Site)

	for i, p := range paths {
		parts := strings.Split(p, string(os.PathSeparator))
		envName := parts[1]
		envs[i] = envName

		t.Environments[envName] = t.ParseConfig(p)
	}
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

func (t *Trellis) Valid() bool {
	return len(t.Environments) > 0
}

func (t *Trellis) EnforceValid(ui cli.Ui) {
	if !t.Valid() {
		ui.Error("No Trellis project detected in the current directory or any of its parent directories.")
		os.Exit(1)
	}
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
