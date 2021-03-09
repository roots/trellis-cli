package cmd

import (
	_ "embed"
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

//go:embed files/sequel_pro_spf_template.xml
var sequelProSpfTemplate string

func (o *DBOpenerSequelPro) Open(c DBCredentials) (err error) {
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

	open := execCommandWithOutput("open", []string{sequelProSpf.Name()}, o.ui)
	if err := open.Run(); err != nil {
		return fmt.Errorf("Error opening database with Tableplus: %s", err)
	}

	return nil
}
