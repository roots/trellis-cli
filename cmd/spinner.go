package cmd

import (
	"time"

	"github.com/theckman/yacspin"
)

type SpinnerCfg struct {
	Message     string
	FailMessage string
	StopMessage string
}

func NewSpinner(config SpinnerCfg) *yacspin.Spinner {
	if config.StopMessage == "" {
		config.StopMessage = config.Message
	}

	cfg := yacspin.Config{
		Frequency:         100 * time.Millisecond,
		CharSet:           yacspin.CharSets[14],
		Suffix:            " ",
		Message:           config.Message,
		SuffixAutoColon:   false,
		StopCharacter:     "[✓]",
		StopColors:        []string{"fgGreen"},
		StopMessage:       config.StopMessage,
		StopFailCharacter: "[✘]",
		StopFailColors:    []string{"fgRed"},
		StopFailMessage:   config.FailMessage,
	}

	spinner, _ := yacspin.New(cfg)

	return spinner
}
