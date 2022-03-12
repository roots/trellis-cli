package trellis

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"log"
	"os"

	"github.com/weppos/publicsuffix-go/publicsuffix"
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

func (s *Site) SslEnabled() bool {
	return s.Ssl["enabled"] == true
}

func (s *Site) MainHost() string {
	return s.SiteHosts[0].Canonical
}

func (s *Site) MainUrl() string {
	var protocol string = "http"

	if s.SslEnabled() {
		protocol = "https"
	}

	return fmt.Sprintf("%s://%s", protocol, s.SiteHosts[0].Canonical)
}

type SiteHost struct {
	Canonical string   `yaml:"canonical"`
	Redirects []string `yaml:"redirects"`
}

type Config struct {
	WordPressSites map[string]*Site `yaml:"wordpress_sites"`
}

func (t *Trellis) ParseConfig(path string) *Config {
	configYaml, err := os.ReadFile(path)

	if err != nil {
		log.Fatalln(err)
	}

	config := &Config{}

	if err = yaml.Unmarshal(configYaml, &config); err != nil {
		log.Fatalln(err)
	}

	return config
}

func (t *Trellis) GenerateSite(site *Site, host string, env string) {
	canonical, redirect := t.HostsFromDomain(host, env)

	if env == "development" {
		site.AdminEmail = fmt.Sprintf("admin@%s", canonical)
		site.Branch = ""
		site.Repo = ""
		site.RepoSubtreePath = ""
	} else {
		site.AdminEmail = ""
	}

	siteHost := SiteHost{
		Canonical: canonical.String(),
	}

	if redirect != nil {
		siteHost.Redirects = []string{redirect.String()}
	}

	site.SiteHosts = []SiteHost{siteHost}
}

func (t *Trellis) HostsFromDomain(domain string, env string) (canonical *publicsuffix.DomainName, redirect *publicsuffix.DomainName) {
	canonical, _ = publicsuffix.Parse(domain)

	if env == "development" {
		canonical.TLD = "test"
	}

	redirect = &publicsuffix.DomainName{canonical.TLD, canonical.SLD, canonical.TRD, &publicsuffix.Rule{}}

	switch canonical.TRD {
	// no subdomain
	case "":
		redirect.TRD = "www"
		return canonical, redirect
	// www subdomain
	case "www":
		redirect.TRD = ""
		return canonical, redirect
	// non-www subdomain
	default:
		return canonical, nil
	}
}

func (t *Trellis) UpdateDefaultConfig(config *Config, name string, host string, env string) {
	config.WordPressSites[name] = config.WordPressSites[DefaultSiteName]

	if name != DefaultSiteName {
		delete(config.WordPressSites, DefaultSiteName)
	}

	t.GenerateSite(config.WordPressSites[name], host, env)
}
