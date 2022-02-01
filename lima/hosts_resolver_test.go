package lima

import (
	"os"
	"path/filepath"
	"testing"
)

type MockInstance struct {
	Name string
}

func (m *MockInstance) HttpHost() string {
	return "http://127.0.0.1:8080"
}

func (m *MockInstance) IP() (string, error) {
	return "192.168.2.1", nil
}

func TestRemoveHostsContent(t *testing.T) {
	tempDir := t.TempDir()
	hostsPath := filepath.Join(tempDir, "hosts")
	hosts := []string{"example.test", "www.example.test"}

	h := HostsFileResolver{
		Hosts:        hosts,
		hostsPath:    filepath.Join(tempDir, "hosts"),
		tmpHostsPath: filepath.Join(tempDir, "hosts.tmp"),
	}

	instance := &MockInstance{Name: "foo-bar"}

	cases := []struct {
		name     string
		content  string
		expected string
	}{
		{
			"no_trellis_block",
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
`,
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
`,
		},
		{
			"trellis_block_end_of_file",
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
## trellis-start-foo-bar
192.168.2.1 example.test www.example.test
## trellis-end-foo-bar
`,
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
`,
		},
		{
			"trellis_block_middle_of_file",
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
## trellis-start-foo-bar
192.168.2.1 example.test www.example.test
## trellis-end-foo-bar
255.255.255.255	broadcasthost
::1             localhost
`,
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
`,
		},
		{
			"trellis_block_multiple_lines",
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
## trellis-start-foo-bar
192.168.2.1 example.test www.example.test
192.168.2.1 new.example.test old.example.test
## trellis-end-foo-bar
255.255.255.255	broadcasthost
::1             localhost
`,
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
`,
		},
		{
			"trellis_block_different_instance",
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
## trellis-start-nope
192.168.2.1 example.test www.example.test
## trellis-end-nope
`,
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
## trellis-start-nope
192.168.2.1 example.test www.example.test
## trellis-end-nope
`,
		},
		{
			"trellis_block_multiple_instances",
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
## trellis-start-foo-bar
192.168.2.1 example.test www.example.test
## trellis-end-foo-bar
## trellis-start-nope
192.168.2.1 example.test www.example.test
## trellis-end-nope
`,
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
## trellis-start-nope
192.168.2.1 example.test www.example.test
## trellis-end-nope
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(hostsPath, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}

			content, err := h.removeHostsContent(instance.Name, instance)
			if err != nil {
				t.Fatal(err)
			}

			if string(content) != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, string(content))
			}
		})
	}
}

func TestAddHostsContent(t *testing.T) {
	tempDir := t.TempDir()
	hostsPath := filepath.Join(tempDir, "hosts")
	hosts := []string{"example.test", "www.example.test"}

	h := HostsFileResolver{
		Hosts:        hosts,
		hostsPath:    filepath.Join(tempDir, "hosts"),
		tmpHostsPath: filepath.Join(tempDir, "hosts.tmp"),
	}

	instance := &MockInstance{Name: "foo-bar"}

	cases := []struct {
		name     string
		content  string
		expected string
	}{
		{
			"no_trellis_block",
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
`,
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
## trellis-start-foo-bar
192.168.2.1 example.test www.example.test
## trellis-end-foo-bar
`,
		},
		{
			"trellis_block_end_of_file",
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
## trellis-start-foo-bar
192.168.99.99 example.test www.example.test
## trellis-end-foo-bar
`,
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
## trellis-start-foo-bar
192.168.2.1 example.test www.example.test
## trellis-end-foo-bar
`,
		},
		{
			"trellis_block_middle_of_file",
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
## trellis-start-foo-bar
192.168.2.1 example.test www.example.test
## trellis-end-foo-bar
255.255.255.255	broadcasthost
::1             localhost
`,
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
## trellis-start-foo-bar
192.168.2.1 example.test www.example.test
## trellis-end-foo-bar
`,
		},
		{
			"trellis_block_different_instance",
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
## trellis-start-nope
192.168.2.1 example.test www.example.test
## trellis-end-nope
`,
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
## trellis-start-nope
192.168.2.1 example.test www.example.test
## trellis-end-nope
## trellis-start-foo-bar
192.168.2.1 example.test www.example.test
## trellis-end-foo-bar
`,
		},
		{
			"trellis_block_multiple_instances",
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
## trellis-start-foo-bar
192.168.2.1 example.test www.example.test
## trellis-end-foo-bar
## trellis-start-nope
192.168.2.1 example.test www.example.test
## trellis-end-nope
`,
			`##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##
127.0.0.1	localhost
255.255.255.255	broadcasthost
::1             localhost
## trellis-start-nope
192.168.2.1 example.test www.example.test
## trellis-end-nope
## trellis-start-foo-bar
192.168.2.1 example.test www.example.test
## trellis-end-foo-bar
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(hostsPath, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}

			content, err := h.addHostsContent(instance.Name, instance)
			if err != nil {
				t.Fatal(err)
			}

			if string(content) != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, string(content))
			}
		})
	}
}
