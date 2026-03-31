package output

import (
	"testing"
)

func TestMergeEnvAddsNew(t *testing.T) {
	env := []string{"HOME=/home/user", "PATH=/usr/bin"}
	result := mergeEnv(env, "ANSIBLE_STDOUT_CALLBACK", "ansible.posix.jsonl")

	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result))
	}

	found := false
	for _, e := range result {
		if e == "ANSIBLE_STDOUT_CALLBACK=ansible.posix.jsonl" {
			found = true
		}
	}
	if !found {
		t.Error("expected ANSIBLE_STDOUT_CALLBACK to be added")
	}
}

func TestMergeEnvReplacesExisting(t *testing.T) {
	env := []string{"HOME=/home/user", "ANSIBLE_STDOUT_CALLBACK=default", "PATH=/usr/bin"}
	result := mergeEnv(env, "ANSIBLE_STDOUT_CALLBACK", "ansible.posix.jsonl")

	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result))
	}

	found := false
	for _, e := range result {
		if e == "ANSIBLE_STDOUT_CALLBACK=ansible.posix.jsonl" {
			found = true
		}
		if e == "ANSIBLE_STDOUT_CALLBACK=default" {
			t.Error("old value should have been replaced")
		}
	}
	if !found {
		t.Error("expected ANSIBLE_STDOUT_CALLBACK to be set to new value")
	}
}

func TestMergeEnvPreservesOther(t *testing.T) {
	env := []string{"HOME=/home/user", "PATH=/usr/bin"}
	result := mergeEnv(env, "FOO", "bar")

	homeFound := false
	pathFound := false
	for _, e := range result {
		if e == "HOME=/home/user" {
			homeFound = true
		}
		if e == "PATH=/usr/bin" {
			pathFound = true
		}
	}

	if !homeFound || !pathFound {
		t.Error("expected existing env vars to be preserved")
	}
}
