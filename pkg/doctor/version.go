package doctor

import (
	"strings"

	"github.com/hashicorp/go-version"
)

// UpstreamVersion strips the epoch and Debian revision/suffix from a dpkg
// version string, e.g. "1:10.11.14+maria~ubu2404" -> "10.11.14" and
// "8.4.18-2+ubuntu24.04.1+deb.sury.org+1" -> "8.4.18".
//
// Ordering of dpkg versions is always done on the server with
// `dpkg --compare-versions`; this is only for display and for comparing
// against upstream release numbers.
func UpstreamVersion(v string) string {
	if i := strings.Index(v, ":"); i >= 0 {
		v = v[i+1:]
	}

	if i := strings.IndexAny(v, "-+~"); i >= 0 {
		v = v[:i]
	}

	return v
}

// Branch returns the "major.minor" part of a version, e.g. "8.3.37" -> "8.3".
func Branch(v string) string {
	parts := strings.SplitN(UpstreamVersion(v), ".", 3)
	if len(parts) < 2 {
		return ""
	}

	return parts[0] + "." + parts[1]
}

// compareVersions compares two plain upstream versions (e.g. "8.3.37").
// ok is false if either can't be parsed.
func compareVersions(a, b string) (cmp int, ok bool) {
	va, errA := version.NewVersion(a)
	vb, errB := version.NewVersion(b)

	if errA != nil || errB != nil {
		return 0, false
	}

	return va.Compare(vb), true
}
