package lima

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

func TestCreateConfig(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	configFile := filepath.Join(dir, "lima.yaml")

	instance := &Instance{
		Dir:        dir,
		ConfigFile: configFile,
		Config: Config{
			Images: []Image{
				{
					Location: "http://ubuntu.com/focal",
					Arch:     "aarch64",
				},
			},
			PortForwards: []PortForward{
				{
					HostPort:  1234,
					GuestPort: 80,
				},
			},
		},
		Sites: trellis.Environments["development"].WordPressSites,
	}

	err := instance.CreateConfig()
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(configFile)

	if err != nil {
		t.Fatal(err)
	}

	absSitePath := filepath.Join(trellis.Path, "../site")

	expected := fmt.Sprintf(`vmType: "vz"
images:
- location: http://ubuntu.com/focal
  arch: aarch64

mounts:
- location: %s
  mountPoint: /srv/www/example.com/current
  writable: true

mountType: "virtiofs"
networks:
- vzNAT: true
portForwards:
- guestPort: 80
  hostPort: 1234

containerd:
  user: false
`, absSitePath)

	if string(content) != expected {
		t.Errorf("expected %s\ngot %s", expected, string(content))
	}
}

func TestCreateInventoryFile(t *testing.T) {
	dir := t.TempDir()

	instance := &Instance{
		Dir:          dir,
		ConfigPath:   dir,
		SshLocalPort: 1234,
		Username:     "dev",
	}

	err := instance.CreateInventoryFile()
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(instance.ConfigPath, "inventory"))

	if err != nil {
		t.Fatal(err)
	}

	expected := `default ansible_host=127.0.0.1 ansible_port=1234 ansible_user=dev ansible_ssh_common_args='-o StrictHostKeyChecking=no'

[development]
default

[web]
default
`

	if string(content) != expected {
		t.Errorf("expected %s\ngot %s", expected, string(content))
	}
}

func TestIP(t *testing.T) {
	instance := &Instance{
		Name: "test",
	}

	mockOutput := `default via 192.168.64.1 proto dhcp src 192.168.64.2 metric 100
192.168.64.0/24 proto kernel scope link src 192.168.64.2
192.168.64.1 proto dhcp scope link src 192.168.64.2 metric 100
`
	commands := []command.MockCommand{
		{
			Command: "limactl",
			Args: []string{
				"shell", "--workdir", "/", instance.Name, "ip", "route", "show", "dev", "lima0",
			},
			Output: mockOutput,
		},
	}
	defer command.MockExecCommands(t, commands)()

	ip, err := instance.IP()
	if err != nil {
		t.Fatal(err)
	}

	expected := "192.168.64.2"

	if ip != expected {
		t.Errorf("expected %s\ngot %s", expected, ip)
	}
}

func TestCommandHelperProcess(t *testing.T) {
	command.CommandHelperProcess(t)
}
