package trellis

import (
	"fmt"
	suffix "golang.org/x/net/publicsuffix"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"strings"
)

const DefaultSiteName = "example.com"

type Site struct {
	SiteHosts       []SiteHost             `yaml:"site_hosts"`
	LocalPath       string                 `yaml:"local_path"`
	AdminEmail      string                 `yaml:"admin_email,omitempty"`
	Branch          string                 `yaml:"branch,omitempty"`
	Repo            string                 `yaml:"repo,omitempty"`
	RepoSubtreePath string                 `yaml:"repo_subtree_path,omitempty"`
	Multisite       map[string]interface{} `yaml:"multisite"`
	Ssl             map[string]interface{} `yaml:"ssl"`
	Cache           map[string]interface{} `yaml:"cache"`
}

type SiteHost struct {
	Canonical string   `yaml:"canonical"`
	Redirects []string `yaml:"redirects"`
}

type Config struct {
	WordPressSites map[string]*Site `yaml:"wordpress_sites"`
}

func (t *Trellis) ParseConfig(path string) *Config {
	configYaml, err := ioutil.ReadFile(path)

	if err != nil {
		log.Fatalln(err)
	}

	config := &Config{}

	if err = yaml.Unmarshal(configYaml, &config); err != nil {
		log.Fatalln(err)
	}

	return config
}

func (t *Trellis) GenerateSite(site *Site, name string, host string, env string) {
	var redirect string

	if env == "development" {
		tld, _ := suffix.PublicSuffix(host)
		host = strings.Replace(host, tld, "test", 1)

		site.AdminEmail = fmt.Sprintf("admin@%s", host)
		site.Branch = ""
		site.Repo = ""
		site.RepoSubtreePath = ""
	} else {
		site.AdminEmail = ""
	}

	if host[:4] == "www." {
		redirect = strings.Replace(host, "www.", "", 1)
	} else {
		redirect = fmt.Sprintf("www.%s", host)
	}

	siteHost := SiteHost{
		Canonical: host,
		Redirects: []string{redirect},
	}

	site.SiteHosts = []SiteHost{siteHost}
}

func (t *Trellis) UpdateDefaultConfig(config *Config, name string, host string, env string) {
	config.WordPressSites[name] = config.WordPressSites[DefaultSiteName]
	delete(config.WordPressSites, DefaultSiteName)
	t.GenerateSite(config.WordPressSites[name], name, host, env)
}

func (t *Trellis) WriteConfigYaml(config *Config, path string) error {
	configYaml, err := yaml.Marshal(config)

	if err != nil {
		log.Fatal(err)
	}

	return t.WriteYamlFile(path, configYaml)
}
