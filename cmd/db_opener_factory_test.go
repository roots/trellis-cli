package cmd

import (
	"strings"
	"testing"
	"reflect"

	"github.com/mitchellh/cli"
)

func TestMakeUnexpected(t *testing.T) {
	factory := &DBOpenerFactory{}

	_, actualErr := factory.make("unexpected-app", cli.NewMockUi())

	actualErrorMessage := actualErr.Error()

	expected := "unexpected-app is not supported"
	if !strings.Contains(actualErrorMessage, expected) {
		t.Errorf("expected command %s to contains %q", actualErr, expected)
	}
}

func TestMakeSequelPro(t *testing.T) {
	factory := &DBOpenerFactory{}

	actual, actualErr := factory.make("sequel-pro", cli.NewMockUi())

	if actualErr != nil {
		t.Errorf("expected error %s to be nil", actualErr)
	}

	actualType := reflect.TypeOf(actual)
	expectedType := reflect.TypeOf(&DBOpenerSequelPro{})

	if actualType != expectedType {
		t.Errorf("expected return type %s to be %s", actualType, expectedType)
	}
}

func TestMakeTableplus(t *testing.T) {
	factory := &DBOpenerFactory{}

	actual, actualErr := factory.make("tableplus", cli.NewMockUi())

	if actualErr != nil {
		t.Errorf("expected error %s to be nil", actualErr)
	}

	actualType := reflect.TypeOf(actual)
	expectedType := reflect.TypeOf(&DBOpenerTableplus{})

	if actualType != expectedType {
		t.Errorf("expected return type %s to be %s", actualType, expectedType)
	}
}
