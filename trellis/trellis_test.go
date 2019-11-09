package trellis

import (
	"fmt"
	"io/ioutil"
	"testing"
	"os"
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
