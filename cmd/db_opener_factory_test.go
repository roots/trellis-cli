package cmd

import (
	"github.com/mitchellh/cli"
	"reflect"
	"strings"
	"testing"
)

func TestMakeUnexpected(t *testing.T) {
	factory := &DBOpenerFactory{}

	_, actualErr := factory.Make("unexpected-app", cli.NewMockUi())

	actualErrorMessage := actualErr.Error()

	expected := "unexpected-app is not supported"
	if !strings.Contains(actualErrorMessage, expected) {
		t.Errorf("expected command %s to contains %q", actualErr, expected)
	}
}

func TestMake(t *testing.T) {
	factory := &DBOpenerFactory{}

	cases := []struct {
		app      string
		expected DBOpener
	}{
		{
			"sequel-ace",
			&DBOpenerSequelAce{},
		},
		{
			"tableplus",
			&DBOpenerTableplus{},
		},
	}

	for _, tc := range cases {
		actual, actualErr := factory.Make(tc.app, cli.NewMockUi())

		if actualErr != nil {
			t.Errorf("expected error %s to be nil", actualErr)
		}

		actualType := reflect.TypeOf(actual)
		expectedType := reflect.TypeOf(tc.expected)

		if actualType != expectedType {
			t.Errorf("expected return type %s to be %s", actualType, expectedType)
		}
	}
}
