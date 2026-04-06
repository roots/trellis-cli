package wsl

import (
	"sort"
	"strings"
)

// UbuntuRootfsURLs maps Ubuntu version numbers to the official cloud-images
// rootfs URLs. These are the WSL-specific images designed for `wsl --import`.
//
// Sources:
//   - 22.04: https://cloud-images.ubuntu.com/wsl/jammy/current/
//   - 24.04: https://cdimages.ubuntu.com/ubuntu-wsl/noble/daily-live/current/
//
// If a URL stops working, the user can manually download a rootfs and place
// it at <configPath>/wsl/ubuntu-rootfs.tar.gz.
var UbuntuRootfsURLs = map[string]string{
	"22.04": "https://cloud-images.ubuntu.com/wsl/jammy/current/ubuntu-jammy-wsl-amd64-ubuntu22.04lts.rootfs.tar.gz",
	"24.04": "https://cdimages.ubuntu.com/ubuntu-wsl/noble/daily-live/current/noble-wsl-amd64.wsl",
}

// supportedUbuntuVersions returns a comma-separated list of supported versions
// for use in error messages.
func supportedUbuntuVersions() string {
	versions := make([]string, 0, len(UbuntuRootfsURLs))
	for v := range UbuntuRootfsURLs {
		versions = append(versions, v)
	}
	sort.Strings(versions)
	return strings.Join(versions, ", ")
}
