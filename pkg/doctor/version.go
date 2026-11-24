package doctor

import (
	"strings"

	"github.com/hashicorp/go-version"
)

// upstreamVersion strips the epoch and Debian revision/suffix from a dpkg
// version string, e.g. "1:10.11.14+maria~ubu2404" -> "10.11.14" and
// "8.4.18-2+ubuntu24.04.1+deb.sury.org+1" -> "8.4.18".
//
// Ordering of dpkg versions is always done on the server with
// `dpkg --compare-versions`; this is only for display and for comparing
// against upstream release numbers.
func upstreamVersion(v string) string {
	if i := strings.Index(v, ":"); i >= 0 {
		v = v[i+1:]
	}

	if i := strings.IndexAny(v, "-+~"); i >= 0 {
		v = v[:i]
	}

	return v
}

// displayVersions returns installed and candidate dpkg versions for
// display: their upstream versions, or, when those are equal (a packaging
// revision bump such as 8.3.37-2 → 8.3.37-3), with the Debian revision too.
func displayVersions(installed, candidate string) (string, string) {
	a, b := upstreamVersion(installed), upstreamVersion(candidate)
	if a != b {
		return a, b
	}

	a, b = withRevision(installed), withRevision(candidate)
	if a != b {
		return a, b
	}

	return installed, candidate
}

// withRevision strips the epoch and distribution suffix, keeping the Debian
// revision: "8.3.37-2+0~deb.sury.org.1+ubuntu24.04" -> "8.3.37-2".
func withRevision(v string) string {
	if i := strings.Index(v, ":"); i >= 0 {
		v = v[i+1:]
	}

	if i := strings.IndexAny(v, "+~"); i >= 0 {
		v = v[:i]
	}

	return v
}

// branchOf returns the "major.minor" part of a version, e.g. "8.3.37" -> "8.3".
func branchOf(v string) string {
	parts := strings.SplitN(upstreamVersion(v), ".", 3)
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
