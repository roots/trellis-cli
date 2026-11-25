package remote_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/roots/trellis-cli/pkg/converge/remote"
)

type limactl map[string]string // command → stdout

func (l limactl) Run(_ context.Context, argv []string, _ io.Reader, stdout, _ io.Writer) error {
	_, _ = io.WriteString(stdout, l[strings.Join(argv[:3], " ")])
	return nil
}

func TestLima(t *testing.T) {
	x := limactl{
		"limactl list dev":  "Running 60022 /Users/me/.lima/dev\n",
		"limactl shell dev": "me\n",
	}
	vm, err := remote.Lima(context.Background(), x, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if vm.Port != 60022 || vm.User != "me" || vm.Key != "/Users/me/.lima/_config/user" {
		t.Errorf("vm = %+v", vm)
	}
	if _, err := remote.Lima(context.Background(), limactl{"limactl list dev": "Stopped 0 /x/dev\n"}, "dev"); err == nil || !strings.Contains(err.Error(), "limactl start dev") {
		t.Errorf("stopped VM: %v", err)
	}
	if _, err := remote.Lima(context.Background(), limactl{}, "nope"); err == nil || !strings.Contains(err.Error(), "no Lima VM") {
		t.Errorf("missing VM: %v", err)
	}
}
