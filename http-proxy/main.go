package httpProxy

import (
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/roots/trellis-cli/app_paths"
)

func AddRecords(proxyHost string, hostNames []string) (err error) {
	hostsPath := hostRecordsPath()

	if err = os.MkdirAll(hostsPath, os.ModePerm); err != nil {
		return err
	}

	recordsNotAdded := []string{}

	for _, host := range hostNames {
		path := filepath.Join(hostsPath, host)
		contents := []byte(proxyHost)
		err = os.WriteFile(path, contents, 0644)

		if err != nil {
			recordsNotAdded = append(recordsNotAdded, path)
		}
	}

	if len(recordsNotAdded) > 0 {
		return fmt.Errorf("Could not add the following proxy records: %s", strings.Join(recordsNotAdded, "\n"))
	}

	return nil
}

func RemoveRecords(hostNames []string) (err error) {
	hostsPath := hostRecordsPath()

	recordsNotDeleted := []string{}

	for _, host := range hostNames {
		path := filepath.Join(hostsPath, host)
		err = os.Remove(path)

		if err != nil {
			recordsNotDeleted = append(recordsNotDeleted, path)
		}
	}

	if len(recordsNotDeleted) > 0 {
		return fmt.Errorf("Could not delete the following proxy records: %s", strings.Join(recordsNotDeleted, "\n"))
	}

	return nil
}

func Run() {
	hostsPath := hostRecordsPath()
	if err := os.MkdirAll(hostsPath, 0744); err != nil {
		log.Fatalln(err)
	}

	tpl := template.Must(template.New("not_found").Parse(NotFoundTemplate))
	h := &proxyHandler{notFoundTemplate: tpl, hostsPath: hostsPath}
	http.Handle("/", h)

	server := &http.Server{Addr: ":80", Handler: h}
	log.Println("Reverse HTTP proxy listening on: 127.0.0.1:80")
	log.Fatal(server.ListenAndServe())
}

func hostRecordsPath() string {
	hostsPath := app_paths.DataDir()
	return filepath.Join(hostsPath, "proxy_hosts")
}
