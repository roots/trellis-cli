package trellis

import (
	"flag"
	"github.com/posener/complete"
	"io"
)

func (t *Trellis) AutocompleteSite(flags *flag.FlagSet) complete.Predictor {
	return t.PredictSite(flags)
}

func (t *Trellis) AutocompleteEnvironment(flags *flag.FlagSet) complete.Predictor {
	return t.PredictEnvironment(flags)
}

func (t *Trellis) PredictSite(flags *flag.FlagSet) complete.PredictFunc {
	return func(args complete.Args) []string {
		if err := t.LoadProject(); err != nil {
			return []string{}
		}

		flags.SetOutput(io.Discard)
		_ = flags.Parse(args.Completed)
		cmdArgs := flags.Args()

		switch len(cmdArgs) {
		case 0:
			return t.EnvironmentNames()
		case 1:
			return t.SiteNamesFromEnvironment(args.LastCompleted)
		default:
			return []string{}
		}
	}
}

func (t *Trellis) PredictEnvironment(flags *flag.FlagSet) complete.PredictFunc {
	return func(args complete.Args) []string {
		if err := t.LoadProject(); err != nil {
			return []string{}
		}

		flags.SetOutput(io.Discard)
		_ = flags.Parse(args.Completed)
		cmdArgs := flags.Args()

		switch len(cmdArgs) {
		case 0:
			return t.EnvironmentNames()
		default:
			return []string{}
		}
	}
}
