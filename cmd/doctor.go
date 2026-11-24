package cmd

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/hashicorp/cli"
	"github.com/mattn/go-isatty"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/app_paths"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/ansible"
	"github.com/roots/trellis-cli/pkg/doctor"
	"github.com/roots/trellis-cli/trellis"
)

// Placeholder host in the hosts files of a new Trellis project.
const placeholderHost = "your_server_hostname"

// doctorUsageError is the exit code when doctor can't run at all.
const doctorUsageError = 2

func NewDoctorCommand(ui cli.Ui, trellis *trellis.Trellis, cliVersion string) *DoctorCommand {
	c := &DoctorCommand{
		UI:         ui,
		Trellis:    trellis,
		CLIVersion: cliVersion,
		stdinIsTerminal: func() bool {
			return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
		},
	}
	c.init()
	return c
}

type DoctorCommand struct {
	UI         cli.Ui
	Trellis    *trellis.Trellis
	CLIVersion string
	flags      *flag.FlagSet
	json       bool
	quiet      bool
	offline    bool
	strict     bool
	fix        bool
	// stdinIsTerminal reports whether --fix can ask for confirmation.
	stdinIsTerminal func() bool
	// Names of the temporary playbook and probe files for this run.
	playbookFile string
	probeFile    string
}

func (c *DoctorCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.json, "json", false, "Output as JSON")
	c.flags.BoolVar(&c.quiet, "q", false, "Only output problems")
	c.flags.BoolVar(&c.quiet, "quiet", false, "Only output problems")
	c.flags.BoolVar(&c.offline, "offline", false, "Skip php.net and GitHub lookups")
	c.flags.BoolVar(&c.strict, "strict", false, "Exit non-zero on warnings too")
	c.flags.BoolVar(&c.fix, "fix", false, "Offer to run the suggested provision commands")
}

func (c *DoctorCommand) Run(args []string) int {
	if err := c.flags.Parse(args); err != nil {
		return doctorUsageError
	}

	args = c.flags.Args()

	if c.fix && c.json {
		c.UI.Error("--fix can't be combined with --json.")
		return doctorUsageError
	}

	if c.fix && !c.stdinIsTerminal() {
		c.UI.Error("--fix asks before running anything, so it needs a terminal. Run the printed `trellis provision` command instead.")
		return doctorUsageError
	}

	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return doctorUsageError
	}

	// Keep --quiet (cron) and --json output to what doctor finds.
	if !c.quiet && !c.json {
		c.Trellis.CheckVirtualenv(c.UI)
	}

	explicit := len(args) > 0
	envs := args
	if !explicit {
		for _, env := range c.Trellis.EnvironmentNames() {
			if env != "development" {
				envs = append(envs, env)
			}
		}
	}

	for _, env := range envs {
		if err := c.Trellis.ValidateEnvironment(env); err != nil {
			c.UI.Error(err.Error())
			if strings.HasPrefix(env, "-") {
				c.UI.Error("Options must come before environment names.")
			}
			return doctorUsageError
		}
	}

	if !c.json && !c.quiet && isatty.IsTerminal(os.Stdout.Fd()) {
		target := "local requirements"
		if len(envs) > 0 {
			target = strings.Join(envs, ", ")
		}
		c.UI.Info(fmt.Sprintf("Checking %s...\n", target))
	}

	report := &doctor.Report{
		Schema: doctor.ReportSchema,
		Local: doctor.LocalChecks(doctor.LocalOptions{
			CLIVersion:     c.CLIVersion,
			TrellisVersion: c.Trellis.Version(),
			Requirements:   trellis.Requirements,
			CacheDir:       app_paths.CacheDir(),
			Offline:        c.offline,
		}),
		Environments: make([]doctor.EnvReport, len(envs)),
	}

	var phpNet *doctor.PHPNet
	var php doctor.PHPReleaseLookup
	if !c.offline {
		phpNet = &doctor.PHPNet{}
		php = phpNet.Release
	}

	if len(envs) > 0 {
		cleanup := c.writePlaybook()
		defer cleanup()

		// A vault password prompt needs the terminal to itself.
		if c.Trellis.CliConfig.AskVaultPass || isTruthy(os.Getenv("ANSIBLE_ASK_VAULT_PASS")) {
			for i, env := range envs {
				report.Environments[i] = c.checkEnv(env, explicit, php)
			}
		} else {
			var wg sync.WaitGroup
			for i, env := range envs {
				wg.Go(func() {
					report.Environments[i] = c.checkEnv(env, explicit, php)
				})
			}
			wg.Wait()
		}
	}

	if phpNet != nil {
		if err := phpNet.Err(); err != nil {
			report.Local = append(report.Local, doctor.Result{
				ID:      "php-net",
				Title:   "php.net",
				Status:  doctor.StatusUnknown,
				Message: "couldn't fetch PHP releases, so PHP wasn't checked against them",
				Details: []string{err.Error()},
			})
		}
	}

	var out bytes.Buffer
	if c.json {
		if err := report.WriteJSON(&out); err != nil {
			c.UI.Error(fmt.Sprintf("Error encoding JSON: %s", err))
			return doctorUsageError
		}
	} else {
		report.WriteText(&out, c.quiet)
	}
	if out.Len() > 0 {
		c.UI.Output(strings.TrimRight(out.String(), "\n"))
	}

	if c.fix && !c.applyFixes(report, php) {
		// A provision run failed, whatever the re-check shows.
		return 1
	}

	return report.ExitCode(c.strict)
}

// applyFixes offers to run each environment's fix, then checks the fixed
// environments again and replaces their results in report. It returns false
// if a provision run failed.
func (c *DoctorCommand) applyFixes(report *doctor.Report, php doctor.PHPReleaseLookup) bool {
	ok := true
	var fixed []int

	for i, env := range report.Environments {
		if env.Fix == nil {
			continue
		}

		c.UI.Output("")
		for _, note := range env.Fix.Notes {
			c.UI.Warn(note)
		}

		answer, err := c.UI.Ask(fmt.Sprintf("Upgrade %s on %s with `trellis provision`? [y/N]", strings.Join(env.Fix.Tags, ", "), env.Name))
		if err != nil {
			// Stop asking (e.g. Ctrl-D), but still check what was fixed.
			c.UI.Error("Canceled; not running anything else.")
			break
		}

		if !isYes(answer) {
			c.UI.Info(fmt.Sprintf("Skipped %s.", env.Name))
			continue
		}

		provision := NewProvisionCommand(c.UI, c.Trellis)
		if code := provision.Run(env.Fix.ProvisionArgs(env.Name)); code != 0 {
			ok = false
			c.UI.Error(fmt.Sprintf("Provisioning %s failed; checking it again to see where it stands.", env.Name))
		}

		fixed = append(fixed, i)
	}

	if len(fixed) == 0 {
		if !slices.ContainsFunc(report.Environments, func(e doctor.EnvReport) bool { return e.Fix != nil }) {
			c.UI.Info("\n--fix has nothing to run.")
		}
		return ok
	}

	recheck := &doctor.Report{Schema: doctor.ReportSchema}
	for _, i := range fixed {
		env := report.Environments[i].Name
		c.UI.Info(fmt.Sprintf("\nChecking %s again...\n", env))
		report.Environments[i] = c.checkEnv(env, true, php)
		recheck.Environments = append(recheck.Environments, report.Environments[i])
	}

	var out bytes.Buffer
	recheck.WriteText(&out, c.quiet)
	if out.Len() > 0 {
		c.UI.Output(strings.TrimRight(out.String(), "\n"))
	}

	return ok
}

func isYes(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes":
		return true
	}
	return false
}

// writePlaybook writes the temporary playbook and probe into the project
// and returns a function that removes them. Names are unique per process so
// concurrent runs don't collide, and the files are also removed on Ctrl-C.
func (c *DoctorCommand) writePlaybook() func() {
	id := os.Getpid()
	c.playbookFile = fmt.Sprintf("%s%d.yml", doctor.FilePrefix, id)
	c.probeFile = fmt.Sprintf("%sprobe_%d.sh", doctor.FilePrefix, id)

	playbook := &AdHocPlaybook{
		path: c.Trellis.Path,
		files: map[string]string{
			c.playbookFile: doctor.PlaybookYml,
			c.probeFile:    doctor.ProbeScript,
		},
	}

	cleanup := sync.OnceFunc(playbook.DumpFiles())

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})

	go func() {
		select {
		case <-signals:
			cleanup()
			os.Exit(130)
		case <-done:
		}
	}()

	return func() {
		signal.Stop(signals)
		close(done)
		cleanup()
	}
}

// checkEnv probes every web host in an environment. Environments that were
// never set up are skipped unless explicitly requested.
func (c *DoctorCommand) checkEnv(env string, explicit bool, php doctor.PHPReleaseLookup) doctor.EnvReport {
	report := doctor.EnvReport{Name: env}

	notReady := func(reason string) doctor.EnvReport {
		report.Status, report.Reason = doctor.EnvSkipped, reason
		if explicit {
			report.Status = doctor.EnvError
		}
		return report
	}

	playbook := ansible.Playbook{Name: c.playbookFile, Env: env}
	playbook.AddExtraVar(doctor.ProbeVar, c.probeFile)

	// Use the same inventory as `trellis provision`.
	inventory := filepath.Join("hosts", env)
	if env == "development" {
		if vmInventory := c.Trellis.VmInventoryPath(); vmInventory != "" {
			inventory = vmInventory
			playbook.SetInventory(vmInventory)
		}
	}

	// A new project's hosts files have a placeholder host. Other inventory
	// layouts (directories, .yml files) are left to Ansible.
	if content, err := os.ReadFile(inventory); err == nil && strings.Contains(string(content), placeholderHost) {
		return notReady(fmt.Sprintf("%s still has the placeholder host %s", inventory, placeholderHost))
	}

	var stdout, stderr bytes.Buffer
	run := command.Cmd("ansible-playbook", playbook.CmdArgs())
	run.Stdout = &stdout
	run.Stderr = &stderr
	// Don't let Ansible consume input meant for --fix's prompts. Password
	// and host key prompts read from the terminal, not stdin.
	run.Stdin = nil
	run.Env = append(run.Environ(), "ANSIBLE_STDOUT_CALLBACK=json")
	if env == "development" {
		run.Env = append(run.Env, "ANSIBLE_HOST_KEY_CHECKING=false")
	}

	runErr := run.Run()

	hosts, err := doctor.ParsePlaybookOutput(stdout.Bytes())
	if errors.Is(err, doctor.ErrNoHosts) {
		return notReady(fmt.Sprintf("no hosts in both the [%s] and [web] groups of the inventory", env))
	}
	if err != nil {
		report.Status, report.Reason = doctor.EnvError, playbookFailure(err, runErr, stderr.String())
		return report
	}

	report.Status = doctor.EnvChecked
	var hostReports []doctor.HostReport
	unchecked := 0

	for _, host := range hosts {
		result := doctor.HostResult{Name: host.Name, Address: host.Address, User: host.User, Error: host.Error}

		if strings.Contains(result.Error, "Host key verification failed") {
			result.Error += fmt.Sprintf("\nConnect once with `trellis ssh %s` to accept the server's host key.", env)
		}

		if result.Error == "" {
			facts, err := doctor.ParseProbe(host.Probe)
			if err != nil {
				result.Error = err.Error()
			} else {
				hostReport := doctor.Evaluate(facts, host.Vars, php)
				result.Results = hostReport.Results
				hostReports = append(hostReports, hostReport)
			}
		}

		if result.Error != "" {
			unchecked++
		}

		report.Hosts = append(report.Hosts, result)
	}

	report.Fix, report.FixNote = doctor.EnvFix(env, hostReports, unchecked)

	return report
}

// playbookFailure explains why ansible-playbook produced no usable output.
func playbookFailure(parseErr, runErr error, stderr string) string {
	// The json stdout callback is in the ansible.posix collection, which the
	// full `ansible` package includes but ansible-core alone doesn't.
	if strings.Contains(stderr, "callback") && strings.Contains(stderr, "json") {
		return "Ansible's json output callback isn't available. It's in the ansible.posix collection, included in the `ansible` package; install it with `ansible-galaxy collection install ansible.posix`."
	}

	reason := parseErr.Error()
	if runErr != nil {
		reason = fmt.Sprintf("ansible-playbook failed (%s)", runErr)
	}
	if msg := lastLines(stderr, 10); msg != "" {
		reason += ":\n" + msg
	}
	return reason
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on", "y":
		return true
	}
	return false
}

func (c *DoctorCommand) Synopsis() string {
	return "Checks whether your servers are patched and up to date"
}

func (c *DoctorCommand) Help() string {
	helpText := `
Usage: trellis doctor [options] [ENVIRONMENT...]

Checks whether your Trellis project and servers are up to date, and prints
the command to fix anything that isn't.

Locally, it checks requirements and whether the project's Trellis is on the
latest release. On each server, it compares the installed Nginx, PHP-FPM and
MariaDB with what's available from their apt sources and php.net, and checks
for a required reboot, unattended-upgrades and the Ubuntu release's support
window.

It connects the same way 'trellis provision' does. Without --fix, it never
uses sudo and changes nothing on your servers. It doesn't check WordPress
core, plugins or TLS certificates.

Check all environments except development:

  $ trellis doctor

Check production only:

  $ trellis doctor production

Only print problems (for cron):

  $ trellis doctor --quiet production

Offer to run the suggested upgrades, then check again:

  $ trellis doctor --fix production

--fix only runs the 'trellis provision' command doctor prints, which
upgrades Nginx, PHP and MariaDB to the newest packages from their current
apt sources. It asks before each environment, never switches PHP or MariaDB
versions, and leaves Ubuntu's own packages and reboots to you.

Arguments:
  ENVIRONMENT  Name of environment(s) (ie: production). Defaults to every
               environment except development; ones that still have the
               placeholder host are skipped.

Options:
      --json     Output as JSON
  -q, --quiet    Only output problems; print nothing when all is well
      --offline  Skip php.net and GitHub lookups
      --strict   Exit with 1 on warnings too
      --fix      Offer to run the suggested provision commands
  -h, --help     Show this help

Options must come before environment names.

Statuses:
  ✓ ok   ! warning   ✗ failed   ? couldn't verify   · info   - skipped

Exit codes:
  0  nothing failed (warnings are allowed without --strict)
  1  a check failed: something is outdated or a requirement is missing
  2  nothing failed, but something couldn't be checked or verified (e.g. an
     unreachable server, or no environment was checked), or doctor couldn't
     run
`

	return strings.TrimSpace(helpText)
}

func (c *DoctorCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteEnvironment(c.flags)
}

func (c *DoctorCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--json":    complete.PredictNothing,
		"--quiet":   complete.PredictNothing,
		"--offline": complete.PredictNothing,
		"--strict":  complete.PredictNothing,
		"--fix":     complete.PredictNothing,
	}
}
