package cmd

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/mitchellh/cli"
	"strings"
	"time"
	"trellis-cli/trellis"
)

type SSLFetchCommand struct {
	UI       cli.Ui
	Trellis  *trellis.Trellis
	playbook PlaybookRunner
}

const sslFetchYmlContent = `
---
- name: Test Connection and Determine Remote User
  hosts: web:&{{ env }}
  gather_facts: false
  roles:
    - { role: connection, tags: [connection, always] }

- name: "Trellis CLI: Pull SSL certificates"
  hosts: web:&{{ env }}
  gather_facts: false
  become: true
  tasks:
    - name: Conditionally 'nginx_path' variable from nginx role
      include_vars:
        file: roles/nginx/defaults/main.yml
      when: nginx_path is not defined

    - name: Fail if couldn't determine 'nginx_path' variable
      fail:
        msg: Failed to load 'nginx_path' variable
      when: nginx_path is not defined

    - name: Find files under SSL directory
      find:
        paths: "{{ nginx_path }}/ssl/"
        recurse: yes
        excludes:
          - no_default.cert
          - no_default.key
          - dhparams.pem
      register: find_result

    - name: Fetch SSL certificates
      fetch:
        src: "{{ item.path }}"
        dest: "{{ dest }}"
      with_items: "{{ find_result.files }}"
`

func NewSSLFetchCommand(ui cli.Ui, trellis *trellis.Trellis) *SSLFetchCommand {
	playbook := &AdHocPlaybook{
		files: map[string]string{
			"ssl-fetch.yml": sslFetchYmlContent,
		},
		Playbook: Playbook{
			ui: ui,
		},
	}

	return &SSLFetchCommand{UI: ui, Trellis: trellis, playbook: playbook}
}

func (c *SSLFetchCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	commandArgumentValidator := &CommandArgumentValidator{required: 1, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	environment := args[0]
	environmentErr := c.Trellis.ValidateEnvironment(environment)
	if environmentErr != nil {
		c.UI.Error(environmentErr.Error())
		return 1
	}

	now := time.Now()
	dest := c.Trellis.Path + "/trellis-cli/ssl/fetch-" + now.Format("20060102150405")

	c.playbook.SetRoot(c.Trellis.Path)

	if err := c.playbook.Run("ssl-fetch.yml", []string{"-e", "env=" + environment, "-e", "dest=" + dest}); err != nil {
		c.UI.Error(fmt.Sprintf("Error running ansible-playbook: %s", err))
		return 1
	}

	c.UI.Info(color.GreenString(fmt.Sprintf("✓ SSL certificates fetched into %s\n", dest)))
	return 0
}

func (c *SSLFetchCommand) Synopsis() string {
	return "Fetch ssl certificates to local system"
}

func (c *SSLFetchCommand) Help() string {
	helpText := `
Usage: trellis ssl fetch [options] ENVIRONMENT

Fetch ssl certificates to local system

Options:
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}
