package trellis

import (
	"github.com/posener/complete"
)

func (t *Trellis) AutocompleteSite() complete.Predictor {
	return t.PredictSite()
}

func (t *Trellis) AutocompleteEnvironment() complete.Predictor {
	return t.PredictEnvironment()
}

func (t *Trellis) PredictSite() complete.PredictFunc {
	return func(args complete.Args) []string {
		if err := t.LoadProject(); err != nil {
			return []string{}
		}

		switch len(args.Completed) {
		case 1:
			return t.EnvironmentNames()
		case 2:
			return t.SiteNamesFromEnvironment(args.LastCompleted)
		default:
			return []string{}
		}
	}
}

func (t *Trellis) PredictEnvironment() complete.PredictFunc {
	return func(args complete.Args) []string {
		if err := t.LoadProject(); err != nil {
			return []string{}
		}

		switch len(args.Completed) {
		case 1:
			return t.EnvironmentNames()
		default:
			return []string{}
		}
	}
}
