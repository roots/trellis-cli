package trust

import "testing"

func TestVerifyResultAllAccounted(t *testing.T) {
	cases := []struct {
		name      string
		result    VerifyResult
		locations []string
		want      bool
	}{
		{
			"all present",
			VerifyResult{Present: []string{"a", "b"}},
			[]string{"a", "b"},
			true,
		},
		{
			"present plus unknown",
			VerifyResult{Present: []string{"a"}, Unknown: []string{"b"}},
			[]string{"a", "b"},
			true,
		},
		{
			"any missing breaks it",
			VerifyResult{Present: []string{"a"}, Missing: []string{"b"}},
			[]string{"a", "b"},
			false,
		},
		{
			"missing alone",
			VerifyResult{Missing: []string{"a"}},
			[]string{"a"},
			false,
		},
		{
			"counts must match locations",
			VerifyResult{Present: []string{"a"}},
			[]string{"a", "b"},
			false,
		},
		{
			"empty locations and result",
			VerifyResult{},
			nil,
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.result.AllAccounted(tc.locations); got != tc.want {
				t.Errorf("AllAccounted = %v, want %v", got, tc.want)
			}
		})
	}
}
