package lxd

import (
	"fmt"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

type MockHostsResolver struct {
	Hosts map[string]string
}

func TestNewManager(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	_, err := NewManager(trellis, cli.NewMockUi())
	if err != nil {
		t.Fatal(err)
	}
}

func TestInitVM(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(trellis, cli.NewMockUi())
	if err != nil {
		t.Fatal(err)
	}

	container := Container{Name: "test"}
	manager.initVM(&container)

	if container.Name != "test" {
		t.Errorf("expected container name to be %q, got %q", "test", container.Name)
	}
}

func TestContainers(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	containerName := "test"
	containersJson := fmt.Sprintf(`[{"architecture":"aarch64","config":{"image.architecture":"arm64","image.description":"ubuntu 22.04 LTS arm64 (release) (20230107)","image.label":"release","image.os":"ubuntu","image.release":"jammy","image.serial":"20230107","image.type":"squashfs","image.version":"22.04","volatile.base_image":"851d46fc056a4a1891de29b32dad2a1fdecebf4961481e2cc0a5c2ee453e49ba","volatile.eth0.host_name":"vethb90b4747","volatile.eth0.hwaddr":"00:16:3e:8c:d3:1d","volatile.idmap.base":"0","volatile.last_state.power":"RUNNING","volatile.uuid":"8f977dc6-09e9-4216-b043-90e4db59b13a"},"devices":{},"ephemeral":false,"profiles":["default","trellis"],"stateful":false,"description":"","created_at":"2023-01-08T17:43:44.088124852Z","name":"%s","status":"Running","status_code":103,"last_used_at":"2023-01-08T17:43:45.681646105Z","location":"none","type":"container","project":"default","backups":null,"state":{"status":"Running","status_code":103,"disk":{"root":{"usage":8689664}},"memory":{"usage":219332608,"usage_peak":260878336,"swap_usage":0,"swap_usage_peak":0},"network":{"eth0":{"addresses":[{"family":"inet","address":"10.99.30.5","netmask":"24","scope":"global"},{"family":"inet6","address":"fd42:8b4f:7529:43f2:216:3eff:fe8c:d31d","netmask":"64","scope":"global"},{"family":"inet6","address":"fe80::216:3eff:fe8c:d31d","netmask":"64","scope":"link"}],"counters":{"bytes_received":117689,"bytes_sent":16441,"packets_received":97,"packets_sent":114,"errors_received":0,"errors_sent":0,"packets_dropped_outbound":0,"packets_dropped_inbound":0},"hwaddr":"00:16:3e:8c:d3:1d","host_name":"vethb90b4747","mtu":1500,"state":"up","type":"broadcast"},"lo":{"addresses":[{"family":"inet","address":"127.0.0.1","netmask":"8","scope":"local"},{"family":"inet6","address":"::1","netmask":"128","scope":"local"}],"counters":{"bytes_received":1712,"bytes_sent":1712,"packets_received":20,"packets_sent":20,"errors_received":0,"errors_sent":0,"packets_dropped_outbound":0,"packets_dropped_inbound":0},"hwaddr":"","host_name":"","mtu":65536,"state":"up","type":"loopback"}},"pid":158889,"processes":38,"cpu":{"usage":6848930922}},"snapshots":null}]`, containerName)

	commands := []command.MockCommand{
		{
			Command: "lxc",
			Args:    []string{"list", "--format=json"},
			Output:  containersJson,
		},
	}

	defer command.MockExecCommands(t, commands)()

	manager, err := NewManager(trellis, cli.NewMockUi())
	if err != nil {
		t.Fatal(err)
	}

	containers := manager.containers()

	if len(containers) != 1 {
		t.Errorf("expected 1 container, got %d", len(containers))
	}

	container, ok := containers[containerName]

	if !ok {
		t.Errorf("expected container with name %s to be present", containerName)
	}

	if container.Name != containerName {
		t.Errorf("expected container name to be %q, got %q", containerName, container.Name)
	}

	expectedIP := "10.99.30.5"
	actualIP, _ := container.IP()

	if actualIP != expectedIP {
		t.Errorf("expected container IP to be %q, got %q", expectedIP, actualIP)
	}
}
