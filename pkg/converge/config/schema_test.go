package config

import (
	"encoding/json"
	"flag"
	"os"
	"slices"
	"strings"
	"testing"
)

var updateSchema = flag.Bool("update-schema", false, "rewrite trellis.schema.json")

func TestSchemaIsCurrent(t *testing.T) {
	b, err := json.MarshalIndent(Schema(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	if *updateSchema {
		if err := os.WriteFile("trellis.schema.json", b, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile("trellis.schema.json")
	if err != nil || string(have) != string(b) {
		t.Fatal("trellis.schema.json is out of date: go test ./pkg/converge/config -run TestSchemaIsCurrent -update-schema")
	}
}

// Every property is described: editors show it, and agents rely on it.
func TestSchemaDescribesEveryProperty(t *testing.T) {
	var missing []string
	walk := func(path string, s map[string]any) {
		props, _ := s["properties"].(map[string]any)
		for k, v := range props {
			p, _ := v.(map[string]any)
			if _, ok := p["description"]; !ok {
				missing = append(missing, path+"."+k)
			}
		}
	}
	root := Schema()
	walk("trellis.yml", root)
	for name, d := range root["$defs"].(map[string]any) {
		walk(name, d.(map[string]any))
	}
	slices.Sort(missing)
	if len(missing) > 0 {
		t.Errorf("properties without a description (add to schemaDescriptions):\n  %s", strings.Join(missing, "\n  "))
	}
}
