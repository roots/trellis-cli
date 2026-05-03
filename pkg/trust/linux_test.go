package trust

import "testing"

func TestSafeFilename(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"alphanumeric", "trellis-abc12345-example.com", "trellis-abc12345-example.com"},
		{"keeps dot dash underscore", "a.b-c_d", "a.b-c_d"},
		{"spaces become underscore", "hello world", "hello_world"},
		{"slashes neutralized", "../../etc/passwd", ".._.._etc_passwd"},
		{"control chars neutralized", "a\nb\tc", "a_b_c"},
		{"unicode is dropped to underscores", "exämple", "ex__mple"},
		{"empty stays empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeFilename(tc.in); got != tc.want {
				t.Errorf("safeFilename(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
