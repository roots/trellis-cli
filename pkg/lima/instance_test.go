package lima

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

func TestGenerateConfig(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()

	absSitePath := filepath.Join(trellis.Path, "../site")
	testCases := []struct {
		name     string
		vmType   string
		expected string
	}{
		{
			name:   "vz",
			vmType: "vz",
			expected: fmt.Sprintf(`vmType: "vz"
rosetta:
  enabled: false
images:
- location: http://ubuntu.com/focal
  arch: aarch64

mounts:
- location: %s
  mountPoint: /srv/www/example.com/current
  writable: true

mountType: "virtiofs"
ssh:
  forwardAgent: true
  loadDotSSHPubKeys: true
networks:
- vzNAT: true

portForwards:
- guestPort: 80
  hostPort: 1234

containerd:
  user: false
provision:
- mode: system
  script: |
    #!/bin/bash
    echo "127.0.0.1 $(hostname)" >> /etc/hosts
`, absSitePath),
		},
		{
			name:   "qemu",
			vmType: "qemu",
			expected: fmt.Sprintf(`vmType: "qemu"
images:
- location: http://ubuntu.com/focal
  arch: aarch64

mounts:
- location: %s
  mountPoint: /srv/www/example.com/current
  writable: true
  9p:
    securityModel: "mapped-xattr"

mountType: "9p"
ssh:
  forwardAgent: true
  loadDotSSHPubKeys: true
networks:
- lima: user-v2

portForwards:
- guestPort: 80
  hostPort: 1234

containerd:
  user: false
provision:
- mode: system
  script: |
    #!/bin/bash
    echo "127.0.0.1 $(hostname)" >> /etc/hosts

    TAP_IFACE="$(ip -o link show | awk '/52:54:00:12:34:56/ {print $2}' | tr -d ':' | head -n1)"
    if [ -n "$TAP_IFACE" ]; then
      ip link set "$TAP_IFACE" up
      ip addr add 192.168.56.5/24 dev "$TAP_IFACE" 2>/dev/null || true
    fi
`, absSitePath),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			instance := &Instance{
				Dir:    dir,
				VMType: tc.vmType,
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

			content, err := instance.GenerateConfig()
			if err != nil {
				t.Fatal(err)
			}

			if content.String() != tc.expected {
				t.Errorf("expected %s\ngot %s", tc.expected, content.String())
			}
		})
	}
}

func TestUpdateConfig(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()

	absSitePath := filepath.Join(trellis.Path, "../site")
	testCases := []struct {
		name     string
		vmType   string
		expected string
	}{
		{
			name:   "vz",
			vmType: "vz",
			expected: fmt.Sprintf(`vmType: "vz"
rosetta:
  enabled: false
images:
- location: http://ubuntu.com/focal
  arch: aarch64

mounts:
- location: %s
  mountPoint: /srv/www/example.com/current
  writable: true

mountType: "virtiofs"
ssh:
  forwardAgent: true
  loadDotSSHPubKeys: true
networks:
- vzNAT: true

portForwards:
- guestPort: 80
  hostPort: 1234

containerd:
  user: false
provision:
- mode: system
  script: |
    #!/bin/bash
    echo "127.0.0.1 $(hostname)" >> /etc/hosts
`, absSitePath),
		},
		{
			name:   "qemu",
			vmType: "qemu",
			expected: fmt.Sprintf(`vmType: "qemu"
images:
- location: http://ubuntu.com/focal
  arch: aarch64

mounts:
- location: %s
  mountPoint: /srv/www/example.com/current
  writable: true
  9p:
    securityModel: "mapped-xattr"

mountType: "9p"
ssh:
  forwardAgent: true
  loadDotSSHPubKeys: true
networks:
- lima: user-v2

portForwards:
- guestPort: 80
  hostPort: 1234

containerd:
  user: false
provision:
- mode: system
  script: |
    #!/bin/bash
    echo "127.0.0.1 $(hostname)" >> /etc/hosts

    TAP_IFACE="$(ip -o link show | awk '/52:54:00:12:34:56/ {print $2}' | tr -d ':' | head -n1)"
    if [ -n "$TAP_IFACE" ]; then
      ip link set "$TAP_IFACE" up
      ip addr add 192.168.56.5/24 dev "$TAP_IFACE" 2>/dev/null || true
    fi
`, absSitePath),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			instance := &Instance{
				Dir:    dir,
				VMType: tc.vmType,
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

			err := instance.UpdateConfig()
			if err != nil {
				t.Fatal(err)
			}

			content, err := os.ReadFile(instance.ConfigFile())
			if err != nil {
				t.Fatal(err)
			}

			if string(content) != tc.expected {
				t.Errorf("expected %s\ngot %s", tc.expected, string(content))
			}
		})
	}
}

func TestCreateInventoryFile(t *testing.T) {
	dir := t.TempDir()

	instance := &Instance{
		Dir:           dir,
		InventoryFile: filepath.Join(dir, "inventory"),
		SshLocalPort:  1234,
		Username:      "dev",
	}

	err := instance.CreateInventoryFile()
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(instance.InventoryFile)

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

func TestHostAccessIPQemu(t *testing.T) {
	instance := &Instance{
		Name:   "test",
		VMType: "qemu",
	}

	mockOutput := `default via 192.168.5.1 dev lima0 proto dhcp src 192.168.5.15 metric 100
192.168.56.0/24 dev enp0s8 proto kernel scope link src 192.168.56.5
`
	commands := []command.MockCommand{
		{
			Command: "limactl",
			Args: []string{
				"shell", "--workdir", "/", instance.Name, "ip", "route", "show",
			},
			Output: mockOutput,
		},
	}
	defer command.MockExecCommands(t, commands)()

	ip, err := instance.HostAccessIP()
	if err != nil {
		t.Fatal(err)
	}

	if ip != "192.168.56.5" {
		t.Errorf("expected 192.168.56.5\ngot %s", ip)
	}
}

func TestCommandHelperProcess(t *testing.T) {
	command.CommandHelperProcess(t)
}
