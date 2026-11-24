package doctor

import "testing"

func TestUpstreamVersion(t *testing.T) {
	cases := map[string]string{
		"8.4.18-2+ubuntu24.04.1+deb.sury.org+1": "8.4.18",
		"1:10.11.14+maria~ubu2404":              "10.11.14",
		"1.29.1-1~noble":                        "1.29.1",
		"1:11.8.3+maria~ubu2604":                "11.8.3",
		"8.3.37":                                "8.3.37",
		"":                                      "",
	}

	for in, want := range cases {
		if got := UpstreamVersion(in); got != want {
			t.Errorf("UpstreamVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBranch(t *testing.T) {
	cases := map[string]string{
		"1:10.11.14+maria~ubu2404": "10.11",
		"8.3.37":                   "8.3",
		"11.8":                     "11.8",
		"8":                        "",
	}

	for in, want := range cases {
		if got := Branch(in); got != want {
			t.Errorf("Branch(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKernelNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"6.8.0-110-generic", "6.8.0-107-generic", true},
		{"6.8.0-107-generic", "6.8.0-110-generic", false},
		{"6.8.0-110-generic", "6.8.0-110-generic", false},
		{"6.11.0-1-generic", "6.8.0-110-generic", true},
		{"", "6.8.0-110-generic", false},
	}

	for _, tc := range cases {
		if got := kernelNewer(tc.a, tc.b); got != tc.want {
			t.Errorf("kernelNewer(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestDisplayVersions(t *testing.T) {
	cases := []struct{ installed, candidate, wantA, wantB string }{
		{"8.3.36-1+ubuntu24.04.1+deb.sury.org+1", "8.3.37-1+ubuntu24.04.1+deb.sury.org+1", "8.3.36", "8.3.37"},
		{"8.3.37-2+0~deb.sury.org.1+ubuntu24.04", "8.3.37-3+debsuryorg24.04.1+1", "8.3.37-2", "8.3.37-3"},
		{"1.31.9-1~noble", "1.31.9-2~noble", "1.31.9-1", "1.31.9-2"},
		{"1:10.11.14+maria~ubu2404", "1:10.11.20+maria~ubu2404", "10.11.14", "10.11.20"},
		{"1.0-1+a", "1.0-1+b", "1.0-1+a", "1.0-1+b"},
	}

	for _, tc := range cases {
		a, b := displayVersions(tc.installed, tc.candidate)
		if a != tc.wantA || b != tc.wantB {
			t.Errorf("displayVersions(%q, %q) = %q, %q; want %q, %q", tc.installed, tc.candidate, a, b, tc.wantA, tc.wantB)
		}
	}
}

func TestSummarizeRebootPackages(t *testing.T) {
	got := summarizeRebootPackages([]string{"libc6", "linux-base", "linux-image-6.8.0-124-generic", "linux-image-6.8.0-149-generic", "linux-image-6.8.0-138-generic"})
	want := "libc6, linux-base, 3 kernel packages (newest linux-image-6.8.0-149-generic)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	if got := summarizeRebootPackages([]string{"libc6", "linux-image-6.8.0-149-generic"}); got != "libc6, linux-image-6.8.0-149-generic" {
		t.Errorf("unexpected short summary %q", got)
	}
}
