package lima

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/roots/trellis-cli/app_paths"
	"github.com/roots/trellis-cli/command"
)

var (
	HostsRemoveErr = errors.New("Error removing hosts")
	HostsAddErr    = errors.New("Error adding hosts")
)

type HostsResolver interface {
	AddHosts(name string, n Networkable) error
	RemoveHosts(name string, n Networkable) error
}

type HostsFileResolver struct {
	Hosts        []string
	hostsPath    string
	tmpHostsPath string
}

func NewHostsResolver(resolverType string, hosts []string) (resolver HostsResolver, err error) {
	switch resolverType {
	case "hosts_file":
		return NewHostsFileResolver(hosts), nil
	default:
		return nil, fmt.Errorf("Unknown hosts resolver type: %s", resolverType)
	}
}

func NewHostsFileResolver(hosts []string) *HostsFileResolver {
	return &HostsFileResolver{
		Hosts:        hosts,
		hostsPath:    "/etc/hosts",
		tmpHostsPath: filepath.Join(app_paths.DataDir(), "hosts"),
	}
}

func (h *HostsFileResolver) AddHosts(name string, n Networkable) error {
	content, err := h.addHostsContent(name, n)
	if err != nil {
		return fmt.Errorf("%w: %v.\nThis is probably a trellis-cli bug; please report it.", HostsAddErr, err)
	}

	return h.writeHostsFile(content)
}

func (h *HostsFileResolver) RemoveHosts(name string, n Networkable) error {
	content, err := h.removeHostsContent(name, n)
	if err != nil {
		return fmt.Errorf("%w: %v.\nThis is probably a trellis-cli bug; please report it.", HostsRemoveErr, err)
	}

	return h.writeHostsFile(content)
}

func (h *HostsFileResolver) addHostsContent(name string, n Networkable) (content []byte, err error) {
	content, err = h.removeHostsContent(name, n)
	if err != nil {
		return []byte{}, err
	}

	instanceHosts, err := h.generateHosts(name, n)
	if err != nil {
		return []byte{}, err
	}

	content = append(content, []byte(instanceHosts)...)
	return content, nil
}

func (h *HostsFileResolver) SudoersCommand() []string {
	return []string{"/bin/cp", h.tmpHostsPath, h.hostsPath}
}

func (h *HostsFileResolver) removeHostsContent(name string, n Networkable) (content []byte, err error) {
	header := fmt.Sprintf("## trellis-start-%s", name)
	footer := fmt.Sprintf("## trellis-end-%s", name)

	re := regexp.MustCompile(fmt.Sprintf(`%s([\s\S]*)%s\n`, header, footer))
	hostsContent, err := os.ReadFile(h.hostsPath)
	if err != nil {
		return []byte{}, fmt.Errorf("Error reading %s file: %v", h.hostsPath, err)
	}

	hostsContent = re.ReplaceAll(hostsContent, []byte{})
	return hostsContent, nil
}

func (h *HostsFileResolver) writeHostsFile(content []byte) error {
	if err := os.WriteFile(h.tmpHostsPath, content, 0644); err != nil {
		return err
	}

	fmt.Printf("\nUpdating %s file (sudo may be required, see `trellis vm sudoers` for more details)\n", h.hostsPath)

	return command.WithOptions(
		command.WithTermOutput(),
	).Cmd("sudo", h.SudoersCommand()).Run()
}

func (h *HostsFileResolver) generateHosts(name string, n Networkable) (string, error) {
	ip, err := n.IP()
	if err != nil {
		return "", err
	}

	content := fmt.Sprintf(`## trellis-start-%s
%s %s
## trellis-end-%s
`, name, ip, strings.Join(h.Hosts, " "), name)

	return content, nil
}
