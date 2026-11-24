package doctor

import (
	"fmt"
	"strings"
)

// EvaluateEnv evaluates an environment's hosts from the doctor playbook's
// output (ansible-playbook with the json stdout callback), including the fix
// for the environment. It returns ErrNoHosts when the inventory has no hosts
// for the environment. Offline skips upstream release lookups.
func EvaluateEnv(env string, playbookOutput []byte, offline bool) (EnvReport, error) {
	runs, err := parsePlaybookOutput(playbookOutput)
	if err != nil {
		return EnvReport{}, err
	}

	report := EnvReport{Name: env, Status: EnvChecked}

	for _, run := range runs {
		host := HostResult{Error: run.Error}
		if host.Error == "" {
			if f, err := parseProbe(run.Probe); err != nil {
				host.Error = err.Error()
			} else {
				host = evaluate(f, run.Vars, offline)
			}
		}

		host.Name, host.Address, host.User = run.Name, run.Address, run.User

		if strings.Contains(host.Error, "Host key verification failed") {
			host.Error += fmt.Sprintf("\nConnect once with `trellis ssh %s` to accept the server's host key.", env)
		}

		report.Hosts = append(report.Hosts, host)
	}

	report.Fix, report.FixNote = envFix(env, report.Hosts)

	return report, nil
}
