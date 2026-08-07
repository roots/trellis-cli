package ansible

import (
	"regexp"
	"sort"
	"strings"
)

/*
Parse output from ansible-playbook --list-tags

Example output:

```
playbook: dev.yml

	play #1 (web:&development): WordPress Server: Install LEMP Stack with PHP and MariaDB MySQL	TAGS: []
	    TASK TAGS: [common, composer, dotenv, fail2ban, ferm, letsencrypt, logrotate, mail, mailhog, mailpit, mariadb, memcached, nginx, nginx-includes, nginx-sites, ntp, php, sshd, wordpress, wordpress-install, wordpress-install-directories, wordpress-setup, wordpress-setup-database, wordpress-setup-nginx, wordpress-setup-nginx-client-cert, wordpress-setup-self-signed-certificate, wp-cli, xdebug]

```
*/
func ParseTags(output string) []string {
	re := regexp.MustCompile(`TASK TAGS:\s*\[([^\]]*)\]`)
	match := re.FindStringSubmatch(output)

	if len(match) < 2 {
		return []string{}
	}

	// Split by comma and trim each tag
	rawTags := strings.Split(match[1], ",")
	var tags []string

	for _, tag := range rawTags {
		trimmed := strings.TrimSpace(tag)

		if trimmed != "" {
			tags = append(tags, trimmed)
		}
	}

	sort.Strings(tags)
	return tags
}
