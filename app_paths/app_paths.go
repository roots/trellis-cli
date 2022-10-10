package app_paths

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	appData          = "AppData"
	trellisConfigDir = "TRELLIS_CONFIG_DIR"
	localAppData     = "LocalAppData"
	xdgCacheHome     = "XDG_CACHE_HOME"
	xdgConfigHome    = "XDG_CONFIG_HOME"
	xdgDataHome      = "XDG_DATA_HOME"
)

// Config path precedence: TRELLIS_CONFIG_DIR, XDG_CONFIG_HOME, AppData (windows only), HOME.
func ConfigDir() string {
	var path string

	if a := os.Getenv(trellisConfigDir); a != "" {
		path = a
	} else if b := os.Getenv(xdgConfigHome); b != "" {
		path = filepath.Join(b, "trellis")
	} else if c := os.Getenv(appData); runtime.GOOS == "windows" && c != "" {
		path = filepath.Join(c, "Trellis CLI")
	} else {
		d, _ := os.UserHomeDir()
		path = filepath.Join(d, ".config", "trellis")
	}

	return path
}

func ConfigPath(path string) string {
	return filepath.Join(ConfigDir(), path)
}

// Cache path precedence: XDG_CACHE_HOME, LocalAppData (windows only), HOME.
func CacheDir() string {
	var path string
	if a := os.Getenv(xdgCacheHome); a != "" {
		path = filepath.Join(a, "trellis")
	} else if b := os.Getenv(localAppData); runtime.GOOS == "windows" && b != "" {
		path = filepath.Join(b, "Trellis CLI")
	} else {
		c, _ := os.UserHomeDir()
		path = filepath.Join(c, ".local", "state", "trellis")
	}
	return path
}
