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
