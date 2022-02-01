package lima

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"os"
	"strings"

	"github.com/roots/trellis-cli/command"
)

//go:embed files/config.yml
var ConfigTemplate string

type Instance struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	Dir          string `json:"dir"`
	Arch         string `json:"arch"`
	Cpus         int    `json:"cpus"`
	Memory       int    `json:"memory"`
	Disk         int    `json:"disk"`
	SshLocalPort int    `json:"sshLocalPort"`
}

func ConvertToInstanceName(value string) string {
	return strings.ReplaceAll(value, ".", "-")
}

func Instances() (instances map[string]Instance) {
	instances = make(map[string]Instance)
	output, err := command.Cmd("limactl", []string{"list", "--json"}).Output()

	for _, line := range strings.Split(string(output), "\n") {
		var instance Instance
		if err = json.Unmarshal([]byte(line), &instance); err == nil {
			instances[instance.Name] = instance
		}
	}

	return instances
}

func GetInstance(name string) (Instance, bool) {
	instances := Instances()
	instance, ok := instances[name]

	return instance, ok
}

func CreateConfig(path string, siteName string) error {
	tpl := template.Must(template.New("lima").Parse(ConfigTemplate))

	file, err := os.Create(path)
	if err != nil {
		return err
	}

	data := struct {
		SiteName string
	}{
		SiteName: siteName,
	}

	err = tpl.Execute(file, data)
	if err != nil {
		return err
	}

	return nil
}
