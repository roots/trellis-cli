package trellis

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/roots/trellis-cli/dns"
	"github.com/roots/trellis-cli/trellis/yamledit"
	"github.com/weppos/publicsuffix-go/publicsuffix"
	"gopkg.in/yaml.v2"
)

const DefaultSiteName = "example.com"

type Site struct {
	SiteHosts       []SiteHost     `yaml:"site_hosts"`
	AbsLocalPath    string         `yaml:"-"`
	LocalPath       string         `yaml:"local_path"`
	AdminEmail      string         `yaml:"admin_email,omitempty"`
	Branch          string         `yaml:"branch,omitempty"`
	Repo            string         `yaml:"repo,omitempty"`
	RepoSubtreePath string         `yaml:"repo_subtree_path,omitempty"`
	Multisite       map[string]any `yaml:"multisite"`
	Ssl             map[string]any `yaml:"ssl"`
	Cache           map[string]any `yaml:"cache"`
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

	for _, site := range config.WordPressSites {
		site.AbsLocalPath = filepath.Join(t.Path, site.LocalPath)
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

	redirect = &publicsuffix.DomainName{
		TLD:  canonical.TLD,
		SLD:  canonical.SLD,
		TRD:  canonical.TRD,
		Rule: &publicsuffix.Rule{},
	}

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

// UpdateDefaultSiteFile renames the default site in a wordpress_sites.yml file
// (relative to the project) and applies the generated hosts and environment
// defaults. Comments and site options not modeled by Site are preserved.
func (t *Trellis) UpdateDefaultSiteFile(path string, name string, host string, env string, header string) error {
	path = filepath.Join(t.Path, path)

	doc, err := yamledit.Load(path)
	if err != nil {
		return err
	}

	if err := doc.RenameKey(name, "wordpress_sites", DefaultSiteName); err != nil {
		return err
	}

	site := &Site{}
	if err := doc.Decode(site, "wordpress_sites", name); err != nil {
		return err
	}

	t.GenerateSite(site, host, env)

	if err := doc.Set(site.SiteHosts, "wordpress_sites", name, "site_hosts"); err != nil {
		return err
	}

	optional := []struct{ key, value string }{
		{"admin_email", site.AdminEmail},
		{"branch", site.Branch},
		{"repo", site.Repo},
		{"repo_subtree_path", site.RepoSubtreePath},
	}

	for _, option := range optional {
		if option.value == "" {
			doc.Delete("wordpress_sites", name, option.key)
		} else if err := doc.Set(option.value, "wordpress_sites", name, option.key); err != nil {
			return err
		}
	}

	if header != "" {
		doc.SetHeadComment(strings.TrimRight(header+"\n"+doc.HeadComment(), "\n"))
	}

	return doc.Save(path)
}

func (s *Site) SslEnabled() bool {
	return s.Ssl["enabled"] == true
}

func (s *Site) SslProvider() string {
	provider, _ := s.Ssl["provider"].(string)
	return provider
}

func (s *Site) MainHost() string {
	return s.SiteHosts[0].Canonical
}

func (s *Site) MainUrl() string {
	var protocol = "http"

	if s.SslEnabled() {
		protocol = "https"
	}

	return fmt.Sprintf("%s://%s", protocol, s.SiteHosts[0].Canonical)
}

func (c *Config) AllHosts() []string {
	hosts := []string{}

	for _, site := range c.WordPressSites {
		for _, siteHost := range site.SiteHosts {
			hosts = append(hosts, siteHost.Canonical)

			hosts = append(hosts, siteHost.Redirects...)
		}
	}

	sort.Strings(hosts)
	return hosts
}

func (c *Config) AllHostsByDomain() map[string][]dns.Host {
	hostsByDomain := map[string][]dns.Host{}

	for _, host := range c.AllHosts() {
		record, _ := dns.ParseHost(host)
		hostsByDomain[record.Domain] = append(hostsByDomain[record.Domain], *record)
	}

	return hostsByDomain
}
