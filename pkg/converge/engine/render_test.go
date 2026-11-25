package engine

import (
	"bytes"
	"strings"
	"testing"

	"github.com/roots/trellis-cli/pkg/converge/manifest"
)

func TestRenderHidesSecrets(t *testing.T) {
	env := manifest.NewFile("/srv/www/a/shared/.env", "DB_PASSWORD='hunter2'", 0o640, "web", "www-data").AsSensitive()
	p := &Plan{Generation: 3, Actions: []Action{
		{Op: OpUpdate, Resource: env, Reasons: []string{"content"}},
		{Op: OpPrune, Resource: manifest.NewPackage("redis-server", "")},
	}, Unchanged: 7, Warnings: []string{"careful"}}
	var buf bytes.Buffer
	p.Render(&buf, "web1")
	out := buf.String()
	if strings.Contains(out, "hunter2") {
		t.Fatal("rendered plan leaked a secret")
	}
	for _, want := range []string{"generation 2 → 3", "~ file", "/srv/www/a/shared/.env 0640 web:www-data   (content)", "× package", "7 unchanged", "warning: careful"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
