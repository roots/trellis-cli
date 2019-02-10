package trellis

import (
	"html/template"
	"os"
	"path/filepath"
)

const Template = `
[{{ .Environment }}]
{{ .Host }}

[web]
{{ .Host }}
`

type Hosts struct {
	Environment string
	Host        string
}

func (t *Trellis) UpdateHosts(env string, ip string) (path string, err error) {
	path = filepath.Join(t.Path, "hosts", env)
	hosts := Hosts{env, ip}
	tpl := template.Must(template.New("hosts").Parse(Template))

	file, err := os.Create(path)
	if err != nil {
		return "", err
	}

	err = tpl.Execute(file, hosts)
	if err != nil {
		return "", err
	}

	return path, nil
}
