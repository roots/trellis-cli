package trust

import "testing"

func TestFormatLocation(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"macos", macOSLoginKeychainLocation, "macOS login keychain"},
		{"linux_user", linuxUserCALocation, "Linux user CA bundle"},
		{"linux_system", linuxSystemCALocation, "Linux system CA bundle"},
		{"nss_unix", "nss:/home/user/.mozilla/firefox/abc.default", "Firefox (abc.default)"},
		{"nss_macos", "nss:/Users/u/Library/Application Support/Firefox/Profiles/xyz.default", "Firefox (xyz.default)"},
		{"unknown_passthrough", "something-else", "something-else"},
		{"nss_no_basename", "nss:", "Firefox (.)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatLocation(tc.in); got != tc.want {
				t.Errorf("FormatLocation(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
