package trellis

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

func TestCreateConfigDir(t *testing.T) {
	dir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(dir)
	configPath := dir + "/testing-trellis-create-config-dir"

	trellis := Trellis{
		ConfigPath: configPath,
	}

	trellis.CreateConfigDir()

	_, err := os.Stat(configPath)
	if err != nil {
		t.Error("expected config directory to be created")
	}
}

func TestEnvironmentNames(t *testing.T) {
	environments := make(map[string]*Config)

	// Intentionally not in alphabetical order.
	environments["b"] = &Config{}
	environments["z"] = &Config{}
	environments["a"] = &Config{}

	trellis := Trellis{
		Environments: environments,
	}

	actual := trellis.EnvironmentNames()

	expected := []string{"a", "b", "z"}

	if fmt.Sprintf("%s", actual) != fmt.Sprintf("%s", expected) {
		t.Errorf("expected %s got %s", expected, actual)
	}
}

func TestValidateEnvironment(t *testing.T) {
	environments := make(map[string]*Config)
	environments["a"] = &Config{}

	trellis := Trellis{
		Environments: environments,
	}

	actual := trellis.ValidateEnvironment("a")
	if actual != nil {
		t.Errorf("expected nil got %s", actual)
	}
}

func TestValidateEnvironmentInvalid(t *testing.T) {
	environments := make(map[string]*Config)
	environments["a"] = &Config{}

	trellis := Trellis{
		Environments: environments,
	}

	actual := trellis.ValidateEnvironment("x")
	if actual == nil {
		t.Error("expected error got nil", actual)
	}
}

func TestSiteNamesFromEnvironment(t *testing.T) {
	environments := make(map[string]*Config)
	environments["a"] = &Config{
		WordPressSites: make(map[string]*Site),
	}

	environments["a"].WordPressSites["a1"] = &Site{}
	environments["a"].WordPressSites["a2"] = &Site{}
	environments["a"].WordPressSites["a3"] = &Site{}

	trellis := Trellis{
		Environments: environments,
	}

	actual := trellis.SiteNamesFromEnvironment("a")

	expected := []string{"a1", "a2", "a3"}

	if fmt.Sprintf("%s", actual) != fmt.Sprintf("%s", expected) {
		t.Errorf("expected %s got %s", expected, actual)
	}
}

func TestFindSiteNameFromEnvironmentDefault(t *testing.T) {
	expected := "a1"

	environments := make(map[string]*Config)
	environments["a"] = &Config{
		WordPressSites: make(map[string]*Site),
	}

	environments["a"].WordPressSites[expected] = &Site{}

	trellis := Trellis{
		Environments: environments,
	}

	actual, actualErr := trellis.FindSiteNameFromEnvironment("a", "")

	if actual != expected {
		t.Errorf("expected %s got %s", expected, actual)
	}

	if actualErr != nil {
		t.Errorf("expected nil got %s", actual)
	}
}

func TestFindSiteNameFromEnvironmentDefaultError(t *testing.T) {
	environments := make(map[string]*Config)
	environments["a"] = &Config{
		WordPressSites: make(map[string]*Site),
	}

	trellis := Trellis{
		Environments: environments,
	}

	actual, actualErr := trellis.FindSiteNameFromEnvironment("a", "")

	if actualErr == nil {
		t.Error("expected error got nil")
	}

	if actual != "" {
		t.Errorf("expected empty string got %s", actual)
	}
}

func TestFindSiteNameFromEnvironmentDefaultErrorMultiple(t *testing.T) {
	environments := make(map[string]*Config)
	environments["a"] = &Config{
		WordPressSites: make(map[string]*Site),
	}

	environments["a"].WordPressSites["a1"] = &Site{}
	environments["a"].WordPressSites["a2"] = &Site{}

	trellis := Trellis{
		Environments: environments,
	}

	actual, actualErr := trellis.FindSiteNameFromEnvironment("a", "")

	if actualErr == nil {
		t.Error("expected error got nil")
	}

	if actual != "" {
		t.Errorf("expected empty string got %s", actual)
	}
}

func TestFindSiteNameFromEnvironment(t *testing.T) {
	expected := "a1"

	environments := make(map[string]*Config)
	environments["a"] = &Config{
		WordPressSites: make(map[string]*Site),
	}

	environments["a"].WordPressSites[expected] = &Site{}

	trellis := Trellis{
		Environments: environments,
	}

	actual, actualErr := trellis.FindSiteNameFromEnvironment("a", expected)

	if actual != expected {
		t.Errorf("expected %s got %s", expected, actual)
	}

	if actualErr != nil {
		t.Errorf("expected nil got %s", actual)
	}
}

func TestFindSiteNameFromEnvironmentInvalid(t *testing.T) {
	environments := make(map[string]*Config)
	environments["a"] = &Config{
		WordPressSites: make(map[string]*Site),
	}

	environments["a"].WordPressSites["a1"] = &Site{}

	trellis := Trellis{
		Environments: environments,
	}

	actual, actualErr := trellis.FindSiteNameFromEnvironment("a", "not-exist")

	if actualErr == nil {
		t.Error("expected error got nil")
	}

	if actual != "" {
		t.Errorf("expected empty string got %s", actual)
	}
}

func TestSiteFromEnvironmentAndName(t *testing.T) {
	expected := &Site{}

	environments := make(map[string]*Config)
	environments["a"] = &Config{
		WordPressSites: make(map[string]*Site),
	}

	environments["a"].WordPressSites["a1"] = &Site{}
	environments["a"].WordPressSites["a2"] = expected
	environments["a"].WordPressSites["a3"] = &Site{}

	trellis := Trellis{
		Environments: environments,
	}

	actual := trellis.SiteFromEnvironmentAndName("a", "a2")

	if actual != expected {
		t.Error("expected site not returned")
	}
}
