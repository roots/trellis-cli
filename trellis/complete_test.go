package trellis

import (
	"strings"
	"testing"

	"github.com/posener/complete"
)

func TestPredictEnvironment(t *testing.T) {
	project := &Project{}
	trellis := NewTrellis(project)

	defer TestChdir(t, "testdata/trellis")()

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
			want:      []string{"development", "production", "valet-link"},
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

	defer TestChdir(t, "testdata/trellis")()

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
			want:          []string{"development", "production", "valet-link"},
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
