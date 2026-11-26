package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/hashicorp/cli"

	"github.com/roots/trellis-cli/app_paths"
	"github.com/roots/trellis-cli/pkg/converge/lock"
	"github.com/roots/trellis-cli/pkg/converge/project"
	"github.com/roots/trellis-cli/pkg/converge/remote"
	"github.com/roots/trellis-cli/pkg/converge/secrets"
	"github.com/roots/trellis-cli/pkg/converge/templates"
	"github.com/roots/trellis-cli/pkg/flags"
)

// The `trellis converge` commands are the experimental Ansible-free engine:
// trellis.yml is compiled locally into per-node manifests, and
// trellis-agent (embedded in trellis-cli) runs on the server to observe,
// plan and apply them.

const convergeHelp = `Usage: trellis converge <subcommand> [<args>]

Experimental: provision servers from trellis.yml without Ansible.

  trellis converge compile production      # show what would be on each node
  trellis converge plan production         # diff against the live servers
  trellis converge apply production        # review, confirm, apply
`

func NewConvergeNamespace() *NamespaceCommand {
	return &NamespaceCommand{
		HelpText:     convergeHelp,
		SynopsisText: "Experimental: provision servers from trellis.yml without Ansible",
	}
}

// parseArgs parses flags anywhere in args (`plan production --host x` and
// `plan --host x production` both work) and returns the positional args.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

// secretKeys finds the user's decryption keys on first use, prompting for
// an SSH key passphrase only if that key can decrypt the file.
func secretKeys(ui cli.Ui) secrets.Keys {
	return secrets.DefaultKeySources(app_paths.ConfigDir(), ui.AskSecret).Keys()
}

func compileEnv(ui cli.Ui, p *project.Project, env, version string) *project.Compiled {
	compiled, err := p.Compile(env, version, secretKeys(ui))
	if err != nil {
		ui.Error(err.Error())
		return nil
	}
	if compiled.PlaintextSecrets != "" {
		ui.Warn(fmt.Sprintf("warning: %s is not encrypted; run `trellis converge secrets rekey %s` before committing", compiled.PlaintextSecrets, env))
	}
	return compiled
}

func openProject(ui cli.Ui) *project.Project {
	p, err := project.Open(".")
	if err != nil {
		ui.Error(err.Error())
		return nil
	}
	return p
}

// --- compile ---------------------------------------------------------------

type ConvergeCompileCommand struct {
	UI      cli.Ui
	Version string
	flags   *flag.FlagSet
	json    bool
	content bool
}

func NewConvergeCompileCommand(ui cli.Ui, version string) *ConvergeCompileCommand {
	c := &ConvergeCompileCommand{UI: ui, Version: version}
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.json, "json", false, "Print the manifest as JSON (includes secrets)")
	c.flags.BoolVar(&c.content, "content", false, "Include file contents (secrets are hidden)")
	return c
}

func (c *ConvergeCompileCommand) Run(args []string) int {
	args, err := parseArgs(c.flags, args)
	if err != nil {
		return 1
	}
	if len(args) != 1 {
		c.UI.Error("Usage: trellis converge compile <environment>")
		return 1
	}
	env := args[0]
	p := openProject(c.UI)
	if p == nil {
		return 1
	}
	compiled := compileEnv(c.UI, p, env, c.Version)
	if compiled == nil {
		return 1
	}
	m := compiled.Manifest
	if c.json {
		b, err := m.Marshal()
		if err != nil {
			c.UI.Error(err.Error())
			return 1
		}
		c.UI.Output(string(b))
		return 0
	}
	var sb strings.Builder
	m.Render(&sb, c.content)
	c.UI.Output(sb.String())
	return 0
}

func (c *ConvergeCompileCommand) Synopsis() string {
	return "Compile trellis.yml into the server's manifest (no server access)"
}

func (c *ConvergeCompileCommand) Help() string {
	return `Usage: trellis converge compile [options] <environment>

Compiles trellis.yml + trellis.lock + secrets into the exact desired state of
the environment's server. Compilation never touches the network or a server, and the same
inputs always produce the same manifest hash.

Options:
  --content  Include file contents (sensitive files are hidden)
  --json     Print the raw manifest JSON (includes secrets!)
`
}

// --- plan / apply ----------------------------------------------------------

type ConvergeRunCommand struct {
	UI      cli.Ui
	Version string
	Apply   bool

	flags *flag.FlagSet
	conn  connection
	yes   bool
}

// connection holds the flags of commands that talk to servers.
type connection struct {
	host        string
	lima        string
	user        string
	sshOptions  flags.StringSliceVar
	agentBinary string
}

func (c *connection) register(fs *flag.FlagSet) {
	fs.StringVar(&c.host, "host", "", "Override the environment's host (e.g. a VM)")
	fs.StringVar(&c.lima, "lima", "", "Target a local Lima VM by name")
	fs.StringVar(&c.user, "user", "", "SSH user (default: admin, falling back to root)")
	fs.Var(&c.sshOptions, "ssh-option", "Extra ssh -o option (repeatable), e.g. ProxyJump=bastion")
	fs.StringVar(&c.agentBinary, "agent-binary", "", "Path to a linux trellis-agent binary (default: the one embedded in trellis-cli)")
}

const connectionHelp = `  --host          Override the environment's host (e.g. a VM)
  --lima          Target a local Lima VM by name
  --user          SSH user (default: admin, falling back to root)
  --ssh-option    Extra ssh -o option, repeatable (e.g. Port=2222)
  --agent-binary  Linux trellis-agent binary (default: embedded)
`

// session connects to compiled's server and makes sure the agent is there.
func (c *connection) session(ctx context.Context, out io.Writer, compiled *project.Compiled) (*remote.Session, *remote.Server, error) {
	ssh := remote.NewSSH()
	ssh.Options = append([]string{"StrictHostKeyChecking=accept-new"}, c.sshOptions...)
	users := []string{"admin", "root"}
	if compiled.Env.User != "" {
		users = []string{compiled.Env.User}
	}
	host, port := c.host, 0
	if c.lima != "" {
		vm, err := remote.Lima(ctx, ssh.Exec, c.lima)
		if err != nil {
			return nil, nil, err
		}
		host, port, users = "127.0.0.1", vm.Port, []string{vm.User}
		ssh.Options = append(vm.Options(), c.sshOptions...)
	}
	if c.user != "" {
		users = []string{c.user}
	}
	s := &remote.Session{
		SSH:          ssh,
		Out:          out,
		Host:         host,
		Port:         port,
		Users:        users,
		SudoPassword: compiled.SudoPassword,
		FindBinary: func(arch string) (remote.Binary, error) {
			return remote.FindBinary(c.agentBinary, arch)
		},
	}
	srv, err := s.Connect(ctx, compiled.Env, compiled.Manifest)
	return s, srv, err
}

func NewConvergeRunCommand(ui cli.Ui, version string, apply bool) *ConvergeRunCommand {
	c := &ConvergeRunCommand{UI: ui, Version: version, Apply: apply}
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.conn.register(c.flags)
	if apply {
		c.flags.BoolVar(&c.yes, "yes", false, "Apply without asking for confirmation")
	}
	return c
}

func (c *ConvergeRunCommand) Run(args []string) int {
	args, err := parseArgs(c.flags, args)
	if err != nil {
		return 1
	}
	if len(args) != 1 {
		c.UI.Error(fmt.Sprintf("Usage: trellis converge %s <environment>", c.verb()))
		return 1
	}
	env := args[0]
	p := openProject(c.UI)
	if p == nil {
		return 1
	}
	ctx, cancel := signalContext()
	defer cancel()

	compiled := compileEnv(c.UI, p, env, c.Version)
	if compiled == nil {
		return 1
	}
	out := &uiWriter{ui: c.UI}
	s, srv, err := c.conn.session(ctx, out, compiled)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	if err := s.Plan(ctx, srv); err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	if srv.Plan.Empty() {
		c.UI.Info("No changes. Servers match trellis.yml.")
		return 0
	}
	if !c.Apply {
		c.UI.Info(fmt.Sprintf("Run `trellis converge apply %s` to apply these changes.", env))
		return 0
	}
	if !c.yes {
		answer, err := c.UI.Ask("Apply these changes? [y/N]")
		if err != nil || !strings.EqualFold(strings.TrimSpace(answer), "y") {
			c.UI.Info("Cancelled.")
			return 1
		}
	}
	if err := s.Apply(ctx, srv); err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	c.UI.Info("Done.")
	return 0
}

func (c *ConvergeRunCommand) verb() string {
	if c.Apply {
		return "apply"
	}
	return "plan"
}

func (c *ConvergeRunCommand) Synopsis() string {
	if c.Apply {
		return "Apply trellis.yml to an environment's servers"
	}
	return "Show what applying trellis.yml would change on the servers"
}

func (c *ConvergeRunCommand) Help() string {
	extra := ""
	if c.Apply {
		extra = "  --yes           Apply without asking for confirmation\n"
	}
	return fmt.Sprintf(`Usage: trellis converge %s [options] <environment>

Compiles the environment, connects to each node over SSH (as admin, falling
back to root on fresh servers), installs the agent if needed and asks it for
a plan. Apply only runs the exact plan you reviewed: if the server changed in
between, it refuses.

Options:
`+connectionHelp+`%s`, c.verb(), extra)
}

// uiWriter adapts cli.Ui to io.Writer, line by line.
type uiWriter struct {
	ui  cli.Ui
	buf []byte
}

func (w *uiWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := strings.IndexByte(string(w.buf), '\n')
		if i < 0 {
			break
		}
		w.ui.Output(string(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

// --- lock ------------------------------------------------------------------

type ConvergeLockCommand struct {
	UI      cli.Ui
	flags   *flag.FlagSet
	refresh bool
}

func NewConvergeLockCommand(ui cli.Ui) *ConvergeLockCommand {
	c := &ConvergeLockCommand{UI: ui}
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.refresh, "refresh", false, "Re-resolve entries that are already locked")
	return c
}

func (c *ConvergeLockCommand) Run(args []string) int {
	if _, err := parseArgs(c.flags, args); err != nil {
		return 1
	}
	p := openProject(c.UI)
	if p == nil {
		return 1
	}
	refs, err := p.KeyRefs()
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	ctx, cancel := signalContext()
	defer cancel()
	l, err := lock.Update(ctx, p.Lock, refs, lock.HTTPKeyFetcher{}, lock.UpdateOptions{Refresh: c.refresh})
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	if err := l.Save(p.LockPath()); err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	c.UI.Info(fmt.Sprintf("Updated %s (%d SSH key references).", p.LockPath(), len(l.SSHKeys)))
	return 0
}

func (c *ConvergeLockCommand) Synopsis() string {
	return "Resolve network references (GitHub SSH keys) into trellis.lock"
}

func (c *ConvergeLockCommand) Help() string {
	return `Usage: trellis converge lock [--refresh]

Resolves everything compilation would otherwise fetch from the network and
pins it in trellis.lock, so compiling is deterministic and offline.
`
}

// --- eject -----------------------------------------------------------------

type ConvergeEjectCommand struct {
	UI      cli.Ui
	Version string
	flags   *flag.FlagSet
	list    bool
	show    bool
}

func NewConvergeEjectCommand(ui cli.Ui, version string) *ConvergeEjectCommand {
	c := &ConvergeEjectCommand{UI: ui, Version: version}
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.list, "list", false, "List built-in templates")
	c.flags.BoolVar(&c.show, "show", false, "Print a built-in template instead of ejecting it")
	return c
}

func (c *ConvergeEjectCommand) Run(args []string) int {
	args, err := parseArgs(c.flags, args)
	if err != nil {
		return 1
	}
	if c.list {
		for _, n := range templates.Names() {
			c.UI.Output(n)
		}
		return 0
	}
	if len(args) != 1 {
		c.UI.Error("Usage: trellis converge eject [--show] <template>")
		return 1
	}
	name := args[0]
	src, err := templates.Source(name)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	if c.show {
		c.UI.Output(string(src))
		return 0
	}
	p := openProject(c.UI)
	if p == nil {
		return 1
	}
	dest := filepath.Join(p.Root, "server", "ejected", name+".tmpl")
	if _, err := os.Stat(dest); err == nil {
		c.UI.Error(dest + " already exists")
		return 1
	} else if !errors.Is(err, fs.ErrNotExist) {
		c.UI.Error(err.Error())
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	if err := os.WriteFile(dest, src, 0o644); err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	sum, _ := templates.SHA256(name)
	if p.Lock.Ejected == nil {
		p.Lock.Ejected = map[string]lock.Ejected{}
	}
	p.Lock.Ejected[name] = lock.Ejected{Base: c.Version, SHA256: sum}
	if err := p.Lock.Save(p.LockPath()); err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	c.UI.Info(fmt.Sprintf("Ejected %s to %s. Trellis will warn when the built-in version changes.", name, dest))
	return 0
}

func (c *ConvergeEjectCommand) Synopsis() string {
	return "Copy a built-in template into the project to override it"
}

func (c *ConvergeEjectCommand) Help() string {
	return `Usage: trellis converge eject [--list | --show] <template>

Copies a built-in template (e.g. nginx/site.conf) to server/ejected/ where it
replaces the built-in one. Prefer drop-ins in server/ when possible: ejected
templates don't get upstream improvements automatically.
`
}

// --- agent (runs on the node) ------------------------------------------------
