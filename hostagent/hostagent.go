package hostagent

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"text/template"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/dns"
	"github.com/roots/trellis-cli/http-proxy"
)

//go:embed files/service.plist
var PlistTemplate string

var (
	ResolverError      = errors.New("Could not create local DNS resolver file")
	LaunchServiceError = errors.New("Could not start service")
)

const (
	DevDomainTld    string = "test"
	DnsResolverPort int    = 41053
	LaunchAgentPath string = "~/Library/LaunchAgents"
	ServiceName     string = "com.roots.trellis"
)

type Port struct {
	Service  string
	Protocol string
	Number   int
}

func Installed() bool {
	serviceErr := command.Cmd("launchctl", []string{"list", ServiceName}).Run()
	_, resolverErr := os.Stat(ResolverPath())

	return serviceErr == nil && resolverErr == nil
}

func Install() (err error) {
	if err = createResolverFile(); err != nil {
		return err
	}

	if err = createPlistFile(); err != nil {
		return err
	}

	var stderr bytes.Buffer
	err = command.WithOptions(
		command.WithCaptureOutput(io.Discard, &stderr),
	).Cmd("launchctl", []string{"bootstrap", launchdDomain(), PlistPath()}).Run()

	if stderr.String() != "" {
		return LaunchServiceError
	}

	return nil
}

func Uninstall() (err error) {
	output, launchctlErr := command.Cmd("launchctl", []string{"bootout", launchdDomain(), PlistPath()}).CombinedOutput()

	if launchctlErr != nil {
		err = fmt.Errorf("Could not uninstall service: %v\n%s", launchctlErr, output)
	}

	if plistErr := os.Remove(PlistPath()); plistErr != nil {
		err = fmt.Errorf("\nCould not remove service plist file: %v", plistErr)
	}

	output, resolverErr := command.Cmd("sudo", []string{"rm", ResolverPath()}).CombinedOutput()

	if resolverErr != nil {
		err = fmt.Errorf("\nCould not remove resolver file: %v\n%s", resolverErr, output)
	}

	return err
}

func PortsInUse() []Port {
	portsInUse := []Port{}

	conn, err := net.DialTimeout("tcp", "[::1]:80", time.Second)
	if err == nil && conn != nil {
		conn.Close()
		portsInUse = append(portsInUse, Port{Service: "HTTP", Protocol: "TCP", Number: 80})
	}

	conn, err = net.DialTimeout("tcp", fmt.Sprintf("[::1]:%d", DnsResolverPort), time.Second)
	if err == nil && conn != nil {
		conn.Close()
		portsInUse = append(portsInUse, Port{Service: "DNS", Protocol: "TCP", Number: DnsResolverPort})
	}

	return portsInUse
}

func Stop() error {
	return command.Cmd("launchctl", []string{"bootout", launchdDomain(), PlistPath()}).Run()
}

func Run() {
	log.Println("trellis-cli started in hostagent mode")
	go runDns()
	go runHttpProxy()

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	s := <-sig
	log.Printf("Signal (%s) received, stopping\n", s)
}

func Running() bool {
	re := regexp.MustCompile(`"PID" = ([0-9]*);`)
	output, err := command.Cmd("launchctl", []string{"list", ServiceName}).Output()

	if err != nil {
		return false
	}

	return re.Match(output)
}

func RunServer() error {
	output, err := command.Cmd("launchctl", []string{"start", ServiceName}).CombinedOutput()

	if err != nil {
		return fmt.Errorf("Error starting hostagent: %v\n%s", err, output)
	}

	time.Sleep(time.Second)

	if !Running() {
		return fmt.Errorf("Error running hostagent")
	}

	return nil
}

func createPlistFile() error {
	tpl := template.Must(template.New("service").Parse(PlistTemplate))
	file, err := os.Create(PlistPath())
	if err != nil {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	data := struct {
		Label   string
		Program string
	}{
		Label:   ServiceName,
		Program: exePath,
	}

	err = tpl.Execute(file, data)
	if err != nil {
		return err
	}

	return nil
}

func createResolverFile() (err error) {
	f, err := os.CreateTemp("", "trellis-cli")
	if err != nil {
		return fmt.Errorf("%w: error creating tmp directory\n%v", ResolverError, err)
	}
	defer os.Remove(f.Name())

	resolverContents := fmt.Sprintf(`nameserver 127.0.0.1
port %d
`, DnsResolverPort)

	if _, err := f.Write([]byte(resolverContents)); err != nil {
		return fmt.Errorf("%w: error writing tmp file\n%v", ResolverError, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("%w: error closing file\n%v", ResolverError, err)
	}

	resolverPath := ResolverPath()
	resolverDir := filepath.Dir(resolverPath)
	mvCommand := fmt.Sprintf("mkdir -p %s && cp %s %s", resolverDir, f.Name(), resolverPath)

	args := []string{
		"sh",
		"-c",
		mvCommand,
	}

	if err = command.Cmd("sudo", args).Run(); err != nil {
		return fmt.Errorf("%w: error writing resolver file %s\n%v", ResolverError, resolverPath, err)
	}

	return nil
}

func PlistPath() string {
	agentPath, _ := homedir.Expand(LaunchAgentPath)
	return filepath.Join(agentPath, ServiceName+".plist")
}

func ResolverPath() string {
	resolverDir := "/etc/resolver"
	return filepath.Join(resolverDir, DevDomainTld)
}

func runDns() error {
	domains := make(map[string]string)
	domains[DevDomainTld] = "127.0.0.1"
	srvOpts := dns.ServerOptions{
		UDPPort: DnsResolverPort,
		TCPPort: DnsResolverPort,
		Address: "127.0.0.1",
		HandlerOptions: dns.HandlerOptions{
			IPv6:          true,
			StaticDomains: domains,
		},
	}

	_, err := dns.Start(srvOpts)

	if err != nil {
		log.Fatalf("cannot start DNS server: %v", err)
	}

	return nil
}

func runHttpProxy() {
	httpProxy.Run()
}

func launchdDomain() string {
	uid := os.Getuid()

	return fmt.Sprintf("gui/%d", uid)
}
