package db_opener

import (
	_ "embed"
	"fmt"
	"os"
	"text/template"
	"time"

	"github.com/roots/trellis-cli/command"
)

type SequelAce struct {
	spfDeleteDelay time.Duration
	spfFile        *os.File
}

//go:embed files/sequel_ace_spf_template.xml
var sequelAceSpfTemplate string

func (o *SequelAce) Open(c DBCredentials) (err error) {
	if o.spfFile == nil {
		sequelAceSpf, sequelAceSpfErr := os.CreateTemp("", "*.spf")
		if sequelAceSpfErr != nil {
			return fmt.Errorf("Error creating temporary SequelAce SPF file: %s", sequelAceSpfErr)
		}

		o.spfFile = sequelAceSpf
	}

	// The SPF file has to be read twice:
	//   1. by the OS to open SequelAce
	//   2. by SequelAce to get db credentials
	// There is a chance that the SPF file got deleted before SequelAce reads the SPF file. We want to delete the SPF file because it contains db credentials in plain text. Therefore, we sleep awhile before deleting the SPF file.
	// 3 seconds is an arbitrary value. It should be enough for most users.
	defer func() {
		time.Sleep(o.spfDeleteDelay)
		os.Remove(o.spfFile.Name())
	}()

	tmpl, tmplErr := template.New("sequelAceSpf").Parse(sequelAceSpfTemplate)
	if tmplErr != nil {
		return fmt.Errorf("Error templating SequelAce SPF: %s", tmplErr)
	}
	if err := tmpl.Execute(o.spfFile, c); err != nil {
		return fmt.Errorf("Error writing SequelAce SPF: %s", err)
	}

	open := command.Cmd("open", []string{o.spfFile.Name()})

	output, err := open.CombinedOutput()

	if err != nil {
		fmt.Println(string(output))
		return fmt.Errorf("Error opening database with SequelAce: %s", err)
	}

	return nil
}
