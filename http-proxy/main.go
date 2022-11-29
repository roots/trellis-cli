package httpProxy

import (
	_ "embed"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"text/template"

	"github.com/roots/trellis-cli/app_paths"
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
	log.Println("Reverse HTTP proxy listening on: 127.0.0.1:80")
	log.Fatal(server.ListenAndServe())
}
