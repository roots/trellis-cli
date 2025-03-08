package trellis

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/roots/trellis-cli/app_paths"
)

func TestCreateConfigDir(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/testing-trellis-create-config-dir"

	trellis := Trellis{
		ConfigDir: configPath,
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

func TestMainSiteFromEnvironment(t *testing.T) {
	expected := &Site{}

	environments := make(map[string]*Config)
	environments["a"] = &Config{
		WordPressSites: make(map[string]*Site),
	}

	environments["a"].WordPressSites["a1"] = expected
	environments["a"].WordPressSites["a2"] = &Site{}
	environments["a"].WordPressSites["a3"] = &Site{}

	trellis := Trellis{
		Environments: environments,
	}

	name, actual, _ := trellis.MainSiteFromEnvironment("a")

	if name != "a1" {
		t.Errorf("expected a1 got %s", name)
	}

	if actual != expected {
		t.Error("expected site not returned")
	}
}

func TestActivateProjectForProjects(t *testing.T) {
	defer LoadFixtureProject(t)()

	tp := NewTrellis()

	if !tp.ActivateProject() {
		t.Error("expected true")
	}

	wd, _ := os.Getwd()

	if tp.Path != wd {
		t.Errorf("expected %s to be %s", tp.Path, wd)
	}

	if tp.Path != wd {
		t.Errorf("expected %s to be %s", tp.Path, wd)
	}
}

func TestActivateProjectForNonProjects(t *testing.T) {
	tempDir := t.TempDir()

	defer TestChdir(t, tempDir)()

	tp := NewTrellis()

	if tp.ActivateProject() {
		t.Error("expected false")
	}
}

func TestActivateProjectForNonVirtualenvInitializedProjects(t *testing.T) {
	defer LoadFixtureProject(t)()
	os.RemoveAll(".trellis/virtualenv")

	tp := NewTrellis()

	if tp.ActivateProject() {
		t.Error("expected false")
	}
}

func TestLoadProjectForProjects(t *testing.T) {
	defer LoadFixtureProject(t)()

	tp := NewTrellis()

	err := tp.LoadProject()
	wd, _ := os.Getwd()

	if err != nil {
		t.Error("expected LoadProject not to return an error")
	}

	if tp.Path != wd {
		t.Errorf("expected %s to be %s", tp.Path, wd)
	}

	expectedEnvNames := []string{"development", "production", "valet-link"}

	if !reflect.DeepEqual(tp.EnvironmentNames(), expectedEnvNames) {
		t.Errorf("expected environment names %s to be %s", tp.EnvironmentNames(), expectedEnvNames)
	}
}

func TestLoadCliConfigWhenFileDoesNotExist(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TRELLIS_CONFIG_DIR", tempDir)

	tp := NewTrellis()
	err := tp.LoadGlobalCliConfig()

	if err != nil {
		t.Error("expected no error")
	}

	if !reflect.DeepEqual(tp.CliConfig, DefaultCliConfig) {
		t.Errorf("expected default CLI config %v, got %v", DefaultCliConfig, tp.CliConfig)
	}
}

func TestLoadGlobalCliConfig(t *testing.T) {
	tp := NewTrellis()

	tempDir := t.TempDir()
	t.Setenv("TRELLIS_CONFIG_DIR", tempDir)

	configFilePath := app_paths.ConfigPath("cli.yml")
	configContents := `
ask_vault_pass: true
`

	if err := os.WriteFile(configFilePath, []byte(configContents), 0666); err != nil {
		t.Fatal(err)
	}

	err := tp.LoadGlobalCliConfig()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tp.CliConfig.AskVaultPass != true {
		t.Errorf("expected global CLI config to get AskVaultPass to true")
	}
}

func TestLoadProjectCliConfig(t *testing.T) {
	defer LoadFixtureProject(t)()

	tp := NewTrellis()

	tempDir := t.TempDir()
	t.Setenv("TRELLIS_CONFIG_DIR", tempDir)

	globalConfigFilePath := app_paths.ConfigPath("cli.yml")
	configContents := `
ask_vault_pass: true
`

	if err := os.WriteFile(globalConfigFilePath, []byte(configContents), 0666); err != nil {
		t.Fatal(err)
	}

	err := tp.LoadGlobalCliConfig()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	projectConfigContents := `
ask_vault_pass: false
`

	if err := os.WriteFile(filepath.Join(tp.Path, "trellis.cli.yml"), []byte(projectConfigContents), 0666); err != nil {
		t.Fatal(err)
	}

	if err = tp.LoadProjectCliConfig(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tp.CliConfig.AskVaultPass != false {
		t.Errorf("expected project CLI config to override AskVaultPass to false")
	}
}

func TestProjectCliConfigIsLoadedFromProjectRoot(t *testing.T) {
	defer LoadFixtureProject(t)()

	tp := NewTrellis()

	configFilePath := filepath.Join(tp.Path, "trellis.cli.yml")

	projectConfigContents := `
ask_vault_pass: true
`

	if err := os.WriteFile(configFilePath, []byte(projectConfigContents), 0666); err != nil {
		t.Fatal(err)
	}

	defer os.Remove(configFilePath)

	// Change directory outside the `trellis` directory to test that
	// `trellis.cli.yml` is still properly loaded
	defer TestChdir(t, "..")()

	err := tp.LoadProject()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tp.CliConfig.AskVaultPass != true {
		t.Errorf("expected load project to load project CLI config file")
	}
}

func TestGetVMInstanceName(t *testing.T) {
	defer LoadFixtureProject(t)()

	tp := NewTrellis()
	err := tp.LoadProject()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test case 1: No instance file and no config setting - should use first site
	instanceName, err := tp.GetVMInstanceName()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedName := tp.SiteNamesFromEnvironment("development")[0]
	if instanceName != expectedName {
		t.Errorf("expected instance name %q, got %q", expectedName, instanceName)
	}

	// Test case 2: With config setting - should use config value
	tp.CliConfig.Vm.InstanceName = "configured-name"
	instanceName, err = tp.GetVMInstanceName()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if instanceName != "configured-name" {
		t.Errorf("expected instance name %q, got %q", "configured-name", instanceName)
	}

	// Test case 3: With instance file - should use file value (highest priority)
	// Create the instance file
	limaDir := filepath.Join(tp.ConfigPath(), "lima")
	if err := os.MkdirAll(limaDir, 0755); err != nil {
		t.Fatalf("failed to create lima directory: %v", err)
	}
	instancePath := filepath.Join(limaDir, "instance")
	if err := os.WriteFile(instancePath, []byte("file-instance-name"), 0644); err != nil {
		t.Fatalf("failed to write instance file: %v", err)
	}

	instanceName, err = tp.GetVMInstanceName()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if instanceName != "file-instance-name" {
		t.Errorf("expected instance name %q, got %q", "file-instance-name", instanceName)
	}

	// Clean up
	os.Remove(instancePath)
	tp.CliConfig.Vm.InstanceName = ""
}

func TestSaveVMInstanceName(t *testing.T) {
	defer LoadFixtureProject(t)()

	tp := NewTrellis()
	err := tp.LoadProject()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test saving the instance name
	err = tp.SaveVMInstanceName("test-instance")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was created and contains correct content
	instancePath := filepath.Join(tp.ConfigPath(), "lima", "instance")
	data, err := os.ReadFile(instancePath)
	if err != nil {
		t.Fatalf("failed to read instance file: %v", err)
	}

	if string(data) != "test-instance" {
		t.Errorf("expected instance file to contain %q, got %q", "test-instance", string(data))
	}

	// Clean up
	os.Remove(instancePath)
}
