package trellis

import (
	"os"
	"testing"
)

func TestUpdateHosts(t *testing.T) {
	defer TestChdir(t, "testdata/trellis")()

	trellis := NewTrellis()

	err := trellis.LoadProject()
	if err != nil {
		t.Fatalf("Could not load Trellis project: %s", err)
	}

	hostsFile, err := trellis.UpdateHosts("production", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(hostsFile)
	if err != nil {
		t.Fatal(err)
	}

	const hostsContent = `
[production]
1.2.3.4

[web]
1.2.3.4
`

	if hostsContent != string(content) {
		t.Errorf("expected hosts contents to be %s, but got %s", hostsContent, string(content))
	}
}
