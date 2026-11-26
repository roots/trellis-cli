package agent_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/roots/trellis-cli/pkg/converge/agent"
	"github.com/roots/trellis-cli/pkg/converge/host/hosttest"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
)

func request(t *testing.T, mode agent.Mode, m *manifest.Manifest, expect string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(agent.Request{Protocol: agent.Protocol, Mode: mode, Manifest: m, Expect: expect}); err != nil {
		t.Fatal(err)
	}
	return &buf
}

func messages(t *testing.T, out *bytes.Buffer) []agent.Message {
	t.Helper()
	var msgs []agent.Message
	sc := bufio.NewScanner(out)
	sc.Buffer(nil, 16<<20)
	for sc.Scan() {
		var m agent.Message
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func testManifest() *manifest.Manifest {
	m := manifest.New("n", []string{"web"})
	_ = m.Add(
		manifest.NewPackage("nginx", ""),
		manifest.NewRootFile("/etc/nginx/nginx.conf", "conf", 0o644, manifest.ReloadUnit("nginx.service", "nginx", "-t")),
		manifest.NewService("nginx.service", manifest.Ptr(true), true),
	)
	m.Finalize()
	return m
}

func TestPlanThenApplyWithApproval(t *testing.T) {
	h := hosttest.New()
	m := testManifest()

	var out bytes.Buffer
	if err := agent.Serve(context.Background(), h, nil, request(t, agent.ModePlan, m, ""), &out); err != nil {
		t.Fatal(err)
	}
	msgs := messages(t, &out)
	if len(msgs) != 1 || msgs[0].Type != "plan" || len(msgs[0].Plan.Actions) != 3 {
		t.Fatalf("plan messages = %+v", msgs)
	}
	approved := msgs[0].Plan.Hash()
	if len(h.Files) != 1 /* /etc/os-release */ {
		t.Fatal("plan mode must not change the host")
	}

	out.Reset()
	if err := agent.Serve(context.Background(), h, nil, request(t, agent.ModeApply, m, approved), &out); err != nil {
		t.Fatal(err)
	}
	msgs = messages(t, &out)
	last := msgs[len(msgs)-1]
	if last.Type != "result" || last.Result.Generation != 1 {
		t.Fatalf("last message = %+v", last)
	}
	events := 0
	for _, msg := range msgs {
		if msg.Type == "event" {
			events++
		}
	}
	if events == 0 {
		t.Error("expected streamed events")
	}

	// A no-op apply doesn't churn generations.
	out.Reset()
	if err := agent.Serve(context.Background(), h, nil, request(t, agent.ModeApply, m, ""), &out); err != nil {
		t.Fatal(err)
	}
	msgs = messages(t, &out)
	if r := msgs[len(msgs)-1].Result; r == nil || r.Generation != 1 {
		t.Errorf("no-op apply result = %+v", msgs[len(msgs)-1])
	}
}

func TestApplyRefusesChangedPlan(t *testing.T) {
	h := hosttest.New()
	m := testManifest()
	var out bytes.Buffer
	_ = agent.Serve(context.Background(), h, nil, request(t, agent.ModePlan, m, ""), &out)
	approved := messages(t, &out)[0].Plan.Hash()

	h.Packages["nginx"] = "1.0" // someone changed the host in between
	out.Reset()
	err := agent.Serve(context.Background(), h, nil, request(t, agent.ModeApply, m, approved), &out)
	if err == nil || !strings.Contains(err.Error(), "changed since the plan was reviewed") {
		t.Fatalf("expected approval mismatch, got %v", err)
	}
	if len(h.Files) != 1 /* /etc/os-release */ {
		t.Error("nothing should be applied")
	}
	if msgs := messages(t, &out); msgs[len(msgs)-1].Type != "error" {
		t.Error("error must be reported on the stream")
	}
}

func TestRejectsProtocolMismatch(t *testing.T) {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(agent.Request{Protocol: 999, Mode: agent.ModePlan, Manifest: testManifest()})
	if err := agent.Serve(context.Background(), hosttest.New(), nil, &buf, &bytes.Buffer{}); err == nil {
		t.Fatal("expected protocol error")
	}
}

func TestRefusesADifferentOS(t *testing.T) {
	h := hosttest.New()
	h.Files["/etc/os-release"].Data = []byte("ID=debian\nVERSION_ID=\"13\"\n")
	m := testManifest()
	m.OS = "ubuntu-24.04"
	var out bytes.Buffer
	err := agent.Serve(context.Background(), h, nil, request(t, agent.ModePlan, m, ""), &out)
	if err == nil || !strings.Contains(err.Error(), "the server runs debian-13 but trellis.yml says os: ubuntu-24.04") {
		t.Fatalf("err = %v", err)
	}
	if got := agent.OSRelease([]byte("NAME=\"Ubuntu\"\nID=ubuntu\nVERSION_ID=\"26.04\"\n")); got != "ubuntu-26.04" {
		t.Errorf("OSRelease = %q", got)
	}
}
