package db_opener

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

type Tableplus struct{}

func (o *Tableplus) Open(c DBCredentials) (err error) {
	uri := o.uriFor(c)

	var open *exec.Cmd
	if runtime.GOOS == "windows" || os.Getenv("WSL_DISTRO_NAME") != "" {
		// Windows or WSL: use rundll32 to open the URI. cmd /c start
		// misparses the & characters in query strings as command separators.
		// rundll32.exe is available inside WSL via Windows interop.
		open = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", uri)
	} else {
		// macOS: use open command.
		open = exec.Command("open", uri)
	}

	// Intentionally omitting `logCmd` to prevent printing db credentials.
	if err := open.Run(); err != nil {
		return fmt.Errorf("Error opening database with Tableplus: %s", err)
	}

	return nil
}

func (o *Tableplus) uriFor(c DBCredentials) string {
	// For WSL development (ansible_connection=local in inventory), the
	// ansible_host will be the inventory hostname "default" — not a real
	// SSH host. MariaDB is accessible directly on localhost via WSL port
	// routing, so use a direct mysql:// URI without SSH.
	if (runtime.GOOS == "windows" || os.Getenv("WSL_DISTRO_NAME") != "") && c.SSHHost == "default" {
		return fmt.Sprintf(
			"mysql://%s:%s@127.0.0.1:3306/%s?enviroment=%s&name=%s&statusColor=F8B502",
			c.DBUser,
			c.DBPassword,
			c.DBName,
			c.WPEnv,
			c.DBName,
		)
	}

	return fmt.Sprintf(
		"mysql+ssh://%s@%s:%d/%s:%s@%s/%s?usePrivateKey=true&enviroment=%s",
		c.SSHUser,
		c.SSHHost,
		c.SSHPort,
		c.DBUser,
		c.DBPassword,
		c.DBHost,
		c.DBName,
		c.WPEnv,
	)
}
