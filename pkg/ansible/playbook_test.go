package ansible

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPlaybook(t *testing.T) {
	playbook := Playbook{
		Name:    "server.yml",
		Env:     "production",
		Verbose: true,
	}

	args := playbook.CmdArgs()

	expected := []string{
		"server.yml",
		"-vvvv",
		"-e env=production",
	}

	if !cmp.Equal(args, expected) {
		t.Errorf("Playbook.CmdArgs() = %v, want %v", args, expected)
	}
}

func TestPlaybookAll(t *testing.T) {
	playbook := Playbook{
		Env: "production",
	}

	playbook.SetName("server.yml")
	playbook.SetInventory("hosts/custom")
	playbook.AddArg("--tags", "users")
	playbook.AddExtraVar("site", "example.com")

	args := playbook.CmdArgs()

	expected := []string{
		"server.yml",
		"--inventory-file=hosts/custom",
		"--tags=users",
		"-e env=production",
		"-e site=example.com",
	}

	if !cmp.Equal(args, expected) {
		t.Errorf("Playbook.CmdArgs() = %v, want %v", args, expected)
	}
}

func TestPlaybookAddExtraVars(t *testing.T) {
	playbook := Playbook{
		Env:     "production",
		Verbose: true,
		ExtraVars: map[string]string{
			"site": "example.com",
		},
	}

	playbook.SetName("server.yml")
	playbook.AddArg("--tags", "users")
	playbook.AddExtraVars("foo=bar")

	args := playbook.CmdArgs()

	expected := []string{
		"server.yml",
		"-vvvv",
		"--tags=users",
		"-e foo=bar",
		"-e env=production",
		"-e site=example.com",
	}

	if !cmp.Equal(args, expected) {
		t.Errorf("Playbook.CmdArgs() = %v, want %v", args, expected)
	}
}
