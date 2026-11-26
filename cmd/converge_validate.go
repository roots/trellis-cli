package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"github.com/hashicorp/cli"

	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/project"
)

// --- validate ----------------------------------------------------------------

type ConvergeValidateCommand struct {
	UI         cli.Ui
	Version    string
	flags      *flag.FlagSet
	json       bool
	configOnly bool
}

func NewConvergeValidateCommand(ui cli.Ui, version string) *ConvergeValidateCommand {
	c := &ConvergeValidateCommand{UI: ui, Version: version}
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.json, "json", false, "Print results as JSON")
	c.flags.BoolVar(&c.configOnly, "config-only", false, "Check trellis.yml only (no secrets, keys or server files)")
	return c
}

// envResult is one environment's validation result (also the JSON shape).
type envResult struct {
	Environment string   `json:"environment"`
	OK          bool     `json:"ok"`
	Errors      []string `json:"errors"`
	Warnings    []string `json:"warnings"`
	Compiled    bool     `json:"compiled"` // false with --config-only or when config is invalid
}

// errorLines turns joined errors into one message per line.
func errorLines(err error) []string {
	var out []string
	for _, l := range strings.Split(err.Error(), "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "compile ") {
			out = append(out, l)
		}
	}
	return out
}

func (c *ConvergeValidateCommand) validate(p *project.Project, env string) envResult {
	r := envResult{Environment: env, Errors: []string{}, Warnings: []string{}}
	if _, err := p.Config.Resolve(env); err != nil {
		r.Errors = errorLines(err)
		return r
	}
	if !c.configOnly {
		compiled, err := p.Compile(env, c.Version, secretKeys(c.UI))
		if err != nil {
			r.Errors = errorLines(err)
			return r
		}
		r.Compiled = true
		r.Warnings = append(r.Warnings, compiled.Manifest.Warnings...)
		if compiled.PlaintextSecrets != "" {
			r.Warnings = append(r.Warnings, compiled.PlaintextSecrets+" is not encrypted")
		}
	}
	r.OK = true
	return r
}

func (c *ConvergeValidateCommand) Run(args []string) int {
	args, err := parseArgs(c.flags, args)
	if err != nil {
		return 1
	}
	p, err := project.Open(".")
	if err != nil {
		if c.json {
			b, _ := json.Marshal(map[string]any{"ok": false, "errors": errorLines(err)})
			c.UI.Output(string(b))
		} else {
			c.UI.Error(err.Error())
		}
		return 1
	}
	envs := args
	if len(envs) == 0 {
		envs = p.Config.EnvironmentNames()
	}
	var results []envResult
	ok := true
	for _, env := range envs {
		r := c.validate(p, env)
		ok = ok && r.OK
		results = append(results, r)
	}
	if c.json {
		b, _ := json.MarshalIndent(map[string]any{"ok": ok, "environments": results}, "", "  ")
		c.UI.Output(string(b))
	} else {
		for _, r := range results {
			mark := "✓"
			if !r.OK {
				mark = "✗"
			}
			c.UI.Output(fmt.Sprintf("%s %s", mark, r.Environment))
			for _, e := range r.Errors {
				c.UI.Output("    error: " + e)
			}
			for _, w := range r.Warnings {
				c.UI.Output("    warning: " + w)
			}
		}
	}
	if !ok {
		return 1
	}
	return 0
}

func (c *ConvergeValidateCommand) Synopsis() string {
	return "Check trellis.yml, secrets and server files for every environment"
}

func (c *ConvergeValidateCommand) Help() string {
	return `Usage: trellis converge validate [options] [<environment>...]

Validates every environment (or the ones given) offline: trellis.yml,
secrets references, keys, server/ files and templates. Exits non-zero on
errors. Needs a key that can decrypt the secrets, unless --config-only.

Options:
  --json         Print results as JSON (for CI and agents)
  --config-only  Check trellis.yml only (no secrets, keys or server files)
`
}

// --- schema ------------------------------------------------------------------

type ConvergeSchemaCommand struct{ UI cli.Ui }

func (c *ConvergeSchemaCommand) Run(args []string) int {
	b, err := json.MarshalIndent(config.Schema(), "", "  ")
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	c.UI.Output(string(b))
	return 0
}

func (c *ConvergeSchemaCommand) Synopsis() string { return "Print the JSON Schema for trellis.yml" }

func (c *ConvergeSchemaCommand) Help() string {
	return `Usage: trellis converge schema

Prints the JSON Schema for trellis.yml. Editors using yaml-language-server
(e.g. VS Code's YAML extension) pick it up from the first line of
trellis.yml:

  # yaml-language-server: $schema=` + config.SchemaID + `
`
}
