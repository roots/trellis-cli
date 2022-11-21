package httpProxy

import (
	"bytes"
	_ "embed"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/roots/trellis-cli/app_paths"
	"github.com/roots/trellis-cli/command"
)

//go:embed files/service.plist
var PlistTemplate string

var (
	PortInUseError    = errors.New("Port 80 already in use")
	LaunchDaemonError = errors.New("Could not start daemon")
)

const (
	ServiceName     string = "com.roots.trellis"
	LaunchAgentPath string = "~/Library/LaunchAgents"
)

func AddRecords(proxyHost string, hostNames []string) (err error) {
	// TODO: allow partial writes
	// TODO: use a subdir just for host records
	hostsPath := app_paths.DataDir()

	for _, host := range hostNames {
		path := filepath.Join(hostsPath, host)
		contents := []byte(proxyHost)
		err = os.WriteFile(path, contents, 0644)

		if err != nil {
			return err
		}
	}

	return nil
}

func RemoveRecords(hostNames []string) (err error) {
	// TODO: allow partial deletes
	hostsPath := app_paths.DataDir()

	for _, host := range hostNames {
		path := filepath.Join(hostsPath, host)
		err = os.Remove(path)

		if err != nil {
			return err
		}
	}

	return nil
}

func Run() {
	hostsPath := app_paths.DataDir()
	if err := os.MkdirAll(hostsPath, 0744); err != nil {
		log.Fatalln(err)
	}

	tpl := template.Must(template.New("not_found").Parse(NotFoundTemplate))
	h := &proxyHandler{notFoundTemplate: tpl, hostsPath: hostsPath}
	http.Handle("/", h)

	server := &http.Server{Addr: ":80", Handler: h}
	log.Println("trellis-cli reverse HTTP proxy listening on 127.0.0.1:80")
	log.Fatal(server.ListenAndServe())
}

func Install() (err error) {
	if err = createPlistFile(); err != nil {
		return err
	}

	processes, err := command.Cmd("launchctl", []string{"list"}).Output()
	if err != nil {
		return err
	}

	for _, process := range strings.Split(string(processes), "\n") {
		if strings.Contains(process, ServiceName) {
			return nil
		}
	}

	conn, err := net.DialTimeout("tcp", ":80", time.Second)
	if err == nil && conn != nil {
		conn.Close()
		return PortInUseError
	}

	var stderr bytes.Buffer
	err = command.WithOptions(
		command.WithCaptureOutput(io.Discard, &stderr),
	).Cmd("launchctl", []string{"load", "-w", plistPath()}).Run()

	if stderr.String() != "" {
		return LaunchDaemonError
	}

	return nil
}

func Stop() error {
	return command.Cmd("launchctl", []string{"unload", plistPath()}).Run()
}

func createPlistFile() error {
	tpl := template.Must(template.New("service").Parse(PlistTemplate))
	file, err := os.Create(plistPath())
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

func plistPath() string {
	agentPath, _ := homedir.Expand(LaunchAgentPath)
	return filepath.Join(agentPath, ServiceName+".plist")
}
