package trellis

import (
	"os"
	"strings"
	"testing"

	"github.com/posener/complete"
)

func testChdir(t *testing.T, dir string) func() {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("err: %s", err)
	}
	return func() { os.Chdir(old) }
}

func TestPredictEnvironment(t *testing.T) {
	project := &Project{}
	trellis := NewTrellis(project)

	defer testChdir(t, "../test-fixtures/trellis")()

	if err := trellis.LoadProject(); err != nil {
		t.Fatalf(err.Error())
	}

	cases := []struct {
		name      string
		completed []string
		want      []string
	}{
		{
			name:      "No args completed",
			completed: []string{},
			want:      []string{},
		},
		{
			name:      "Command supplied",
			completed: []string{"command"},
			want:      []string{"development", "production"},
		},
		{
			name:      "Command and env supplied",
			completed: []string{"command", "development"},
			want:      []string{},
		},
	}

	for _, tc := range cases {
		matches := trellis.PredictEnvironment().Predict(
			complete.Args{Completed: tc.completed},
		)

		got := strings.Join(matches, ",")
		want := strings.Join(tc.want, ",")

		if got != want {
			t.Errorf("failed %s\ngot = %s\nwant: %s", t.Name(), got, want)
		}
	}
}

func TestPredictSite(t *testing.T) {
	project := &Project{}
	trellis := NewTrellis(project)

	defer testChdir(t, "../test-fixtures/trellis")()

	if err := trellis.LoadProject(); err != nil {
		t.Fatalf(err.Error())
	}

	cases := []struct {
		name          string
		completed     []string
		lastCompleted string
		want          []string
	}{
		{
			name:          "No args completed",
			completed:     []string{},
			lastCompleted: "",
			want:          []string{},
		},
		{
			name:          "Command supplied",
			completed:     []string{"command"},
			lastCompleted: "command",
			want:          []string{"development", "production"},
		},
		{
			name:          "Command and env supplied",
			completed:     []string{"command", "development"},
			lastCompleted: "development",
			want:          []string{"example.com"},
		},
		{
			name:          "Command, env, and site supplied",
			completed:     []string{"command", "development", "example.com"},
			lastCompleted: "example.com",
			want:          []string{},
		},
	}

	for _, tc := range cases {
		matches := trellis.PredictSite().Predict(
			complete.Args{Completed: tc.completed, LastCompleted: tc.lastCompleted},
		)

		got := strings.Join(matches, ",")
		want := strings.Join(tc.want, ",")

		if got != want {
			t.Errorf("failed %s\ngot = %s\nwant: %s", t.Name(), got, want)
		}
	}
}
