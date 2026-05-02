package trust

import (
	"path/filepath"
	"strings"
)

// FormatLocation maps an internal location identifier to a human-readable
// label suitable for CLI output. Internal forms like
// "nss:/Users/.../Library/Application Support/Firefox/Profiles/abc.default"
// become "Firefox (abc.default)".
func FormatLocation(loc string) string {
	switch loc {
	case macOSLoginKeychainLocation:
		return "macOS login keychain"
	case linuxUserCALocation:
		return "Linux user CA bundle"
	case linuxSystemCALocation:
		return "Linux system CA bundle"
	}
	const nssPrefix = "nss:"
	if strings.HasPrefix(loc, nssPrefix) {
		return "Firefox (" + filepath.Base(loc[len(nssPrefix):]) + ")"
	}
	return loc
}
