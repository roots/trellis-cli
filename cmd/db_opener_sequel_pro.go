package cmd

import (
	"fmt"

	"io/ioutil"
	"text/template"

	"github.com/mitchellh/cli"
)

type DBOpenerSequelPro struct {
	ui cli.Ui
}

var sequelProSpfTemplate = `
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
	// TODO: [Help Wanted] There is a chance that the SPF file got deleted before SequelPro establish db connection.
	//       But we really want to delete this file because it contains db credentials in plain text.
	//defer deleteFile(sequelProSpf.Name())

	tmpl, tmplErr := template.New("sequelProSpf").Parse(sequelProSpfTemplate)
	if tmplErr != nil {
		return fmt.Errorf("Error templating SequelPro SPF: %s", tmplErr)
	}

	tmplExecuteErr := tmpl.Execute(sequelProSpf, c)
	if tmplExecuteErr != nil {
		return fmt.Errorf("Error writing SequelPro SPF: %s", tmplExecuteErr)
	}

	open := execCommand("open", sequelProSpf.Name())
	logCmd(open, o.ui, true)
	openErr := open.Run()
	if openErr != nil {
		return fmt.Errorf("Error opening database with Tableplus: %s", openErr)
	}

	return nil
}
