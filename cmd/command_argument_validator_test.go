package cmd

import (
	"strings"
	"testing"
)

func TestCommandArgumentValidatorValidate(t *testing.T) {
	cases := []struct {
		name             string
		args             []string
		requiredArgCount int
		optionalArgCount int
		expectedMessage  string
	}{
		{
			"valid_without_optional",
			[]string{"a", "b", "c"},
			3,
			0,
			"",
		},
		{
			"valid_with_optional",
			[]string{"a", "b", "c"},
			1,
			2,
			"",
		},
		{
			"missing_without_optional",
			[]string{"a", "b", "c"},
			4,
			0,
			"missing arguments (expected exactly 4, got 3)",
		},
		{
			"missing_with_optional",
			[]string{"a", "b", "c"},
			4,
			2,
			"missing arguments (expected between 4 and 6, got 3)",
		},
		{
			"too_many_without_optional",
			[]string{"a", "b", "c"},
			2,
			0,
			"too many arguments (expected exactly 2, got 3)",
		},
		{
			"too_many_with_optional",
			[]string{"a", "b", "c"},
			1,
			1,
			"too many arguments (expected between 1 and 2, got 3)",
		},
	}

	for _, tc := range cases {
		validator := &CommandArgumentValidator{
			required: tc.requiredArgCount,
			optional: tc.optionalArgCount,
		}

		actual := validator.validate(tc.args)

		if "" == tc.expectedMessage && actual != nil {
			t.Errorf("expected result to be valid, got %s", actual)
		}

		if "" != tc.expectedMessage && actual == nil {
			t.Errorf("expected result to be invalid, got nil")
		}

		if "" != tc.expectedMessage && actual != nil {
			if !strings.Contains(actual.Error(), tc.expectedMessage) {
				t.Errorf("expected error to contains %s, got %s", tc.expectedMessage, actual)
			}
		}
	}
}
