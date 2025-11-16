package ansible

import (
	"fmt"
	"sort"
)

type Playbook struct {
	Name      string
	Env       string
	Verbose   bool
	ExtraVars map[string]string
	args      []string
}

func (p *Playbook) AddArg(name string, value string) *Playbook {
	p.args = append(p.args, name+"="+value)
	return p
}

func (p *Playbook) AddFlag(name string) *Playbook {
	p.args = append(p.args, name)
	return p
}

func (p *Playbook) AddExtraVar(name string, value string) *Playbook {
	if p.ExtraVars == nil {
		p.ExtraVars = make(map[string]string)
	}

	p.ExtraVars[name] = value
	return p
}

func (p *Playbook) AddExtraVars(extraVars string) *Playbook {
	p.args = append(p.args, fmt.Sprintf("-e %s", extraVars))
	return p
}

func (p *Playbook) SetInventory(path string) *Playbook {
	if path != "" {
		p.AddArg("--inventory", path)
	}

	return p
}

func (p *Playbook) SetName(name string) *Playbook {
	p.Name = name
	return p
}

func (p *Playbook) CmdArgs() []string {
	args := []string{p.Name}

	if p.Verbose {
		args = append(args, "-vvvv")
	}

	args = append(args, p.args...)

	if p.Env != "" {
		p.AddExtraVar("env", p.Env)
	}

	keys := make([]string, 0, len(p.ExtraVars))
	for key := range p.ExtraVars {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, k := range keys {
		args = append(args, fmt.Sprintf("-e %s=%s", k, p.ExtraVars[k]))
	}

	return args
}
