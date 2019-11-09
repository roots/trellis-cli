package trellis

import (
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
