package cmd

import (
	"fmt"
	"github.com/mitchellh/cli"
	"io/ioutil"
	"os"
	"text/template"
	"time"
)

type DBOpenerSequelPro struct {
	ui cli.Ui
}

const sequelProSpfTemplate = `
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>ContentFilters</key>
  <dict/>
  <key>auto_connect</key>
  <true/>
  <key>data</key>
  <dict>
    <key>connection</key>
    <dict>
      <key>database</key>
      <string>{{.DBName}}</string>
      <key>host</key>
      <string>{{.DBHost}}</string>
      <key>user</key>
      <string>{{.DBUser}}</string>
      <key>password</key>
      <string>{{.DBPassword}}</string>
      <key>ssh_host</key>
      <string>{{.SSHHost}}</string>
      <key>ssh_port</key>
      <string>{{.SSHPort}}</string>
      <key>ssh_user</key>
      <string>{{.SSHUser}}</string>
      <key>type</key>
      <string>SPSSHTunnelConnection</string>
    </dict>
  </dict>
  <key>format</key>
  <string>connection</string>
  <key>queryFavorites</key>
  <array/>
  <key>queryHistory</key>
  <array/>
</dict>
</plist>
`

func (o *DBOpenerSequelPro) open(c DBCredentials) (err error) {
	sequelProSpf, sequelProSpfErr := ioutil.TempFile("", "*.spf")
	if sequelProSpfErr != nil {
		return fmt.Errorf("Error creating temporary SequelPro SPF file: %s", sequelProSpfErr)
	}

	// The SPF file has to be read twice:
	//   1. by the OS to open SequelPro
	//   2. by SequelPro to get db credentials
	// There is a chance that the SPF file got deleted before SequelPro reads the SPF file.
	// We want to delete the SPF file because it contains db credentials in plain text.
	// Therefore, we sleep awhile before deleting the SPF file.
	// 3 seconds is an arbitrary value. It should be enough for most users.
	defer func() {
		time.Sleep(3 * time.Second)
		os.Remove(sequelProSpf.Name())
	}()

	tmpl, tmplErr := template.New("sequelProSpf").Parse(sequelProSpfTemplate)
	if tmplErr != nil {
		return fmt.Errorf("Error templating SequelPro SPF: %s", tmplErr)
	}
	if err := tmpl.Execute(sequelProSpf, c); err != nil {
		return fmt.Errorf("Error writing SequelPro SPF: %s", err)
	}

	open := execCommand("open", sequelProSpf.Name())
	logCmd(open, o.ui, true)
	if err := open.Run(); err != nil {
		return fmt.Errorf("Error opening database with Tableplus: %s", err)
	}

	return nil
}
