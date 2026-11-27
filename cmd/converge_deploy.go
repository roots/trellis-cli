package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/cli"

	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/deploy"
	"github.com/roots/trellis-cli/pkg/converge/project"
	"github.com/roots/trellis-cli/pkg/converge/remote"
)

// --- deploy ------------------------------------------------------------------

type ConvergeDeployCommand struct {
	UI      cli.Ui
	Version string
	flags   *flag.FlagSet
	conn    connection
	ref     string
}

func NewConvergeDeployCommand(ui cli.Ui, version string) *ConvergeDeployCommand {
	c := &ConvergeDeployCommand{UI: ui, Version: version}
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.ref, "ref", "", "Branch, tag or commit to deploy, as your clone has it (default: the site's branch on its remote; in CI, the checked-out commit)")
	c.conn.register(c.flags)
	return c
}

// resolveEnv reads env's settings from trellis.yml. Deploys need nothing
// more: no secrets, no compile.
func resolveEnv(ui cli.Ui, p *project.Project, env string) *config.Environment {
	e, err := p.Config.Resolve(env)
	if err != nil {
		ui.Error(err.Error())
		return nil
	}
	return e
}

// findSite returns site name in env.
func findSite(env *config.Environment, name string) (config.Site, error) {
	for _, s := range env.Sites {
		if s.Name == name {
			return s, nil
		}
	}
	var names []string
	for _, s := range env.Sites {
		names = append(names, s.Name)
	}
	return config.Site{}, fmt.Errorf("site %q is not in %s (sites: %v)", name, env.Name, names)
}

func (c *ConvergeDeployCommand) Run(args []string) int {
	args, err := parseArgs(c.flags, args)
	if err != nil {
		return 1
	}
	if len(args) != 2 {
		c.UI.Error("Usage: trellis converge deploy [options] <environment> <site>")
		return 1
	}
	env, name := args[0], args[1]
	p := openProject(c.UI)
	if p == nil {
		return 1
	}
	e := resolveEnv(c.UI, p, env)
	if e == nil {
		return 1
	}
	if err := c.deploy(p, e, name); err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	return 0
}

func (c *ConvergeDeployCommand) deploy(p *project.Project, e *config.Environment, name string) error {
	ctx, cancel := signalContext()
	defer cancel()
	site, err := findSite(e, name)
	if err != nil {
		return err
	}
	hooks, local, err := deploy.Hooks(p.Root)
	if err != nil {
		return err
	}

	start := time.Now()
	source := site.Source
	if !filepath.IsAbs(source) {
		source = filepath.Join(p.Root, source)
	}
	repo, subtree, err := deploy.Locate(ctx, source, site.Repo)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	src, err := deploy.Resolve(ctx, repo, c.ref, site.Branch, os.Getenv("CI") == "true")
	if err != nil {
		return err
	}
	for _, n := range src.Notes {
		c.UI.Warn("Note: " + n)
	}
	ref := src.From
	c.UI.Info(fmt.Sprintf("Building %s at %s (%s)…", name, src.From, src.Commit[:7]))
	var buildOut bytes.Buffer
	art, err := deploy.Build(ctx, deploy.BuildOptions{
		Repo:    repo,
		Ref:     src.Commit,
		Subtree: subtree,
		Hook:    local.Build,
		Env:     []string{"TRELLIS_ENV=" + e.Name, "TRELLIS_SITE=" + name},
		Exclude: site.Deploy.Exclude,
		Out:     &buildOut,
	})
	if err != nil {
		if buildOut.Len() > 0 {
			c.UI.Error(buildOut.String())
		}
		return err
	}
	c.UI.Info(fmt.Sprintf("Built %s: %d files, %.1f MB (%s)", art.Commit[:7], art.Files, float64(len(art.Data))/1e6, time.Since(start).Round(100*time.Millisecond)))
	for _, w := range art.Warnings {
		c.UI.Warn("Warning: " + w)
	}
	c.UI.Info("")

	spec := deploy.Spec(e, site, art, hooks, time.Now())
	s, srv, err := c.conn.deploySession(ctx, &uiWriter{ui: c.UI}, e)
	if err != nil {
		return err
	}
	if err := s.Deploy(ctx, srv, remote.Artifact{Data: art.Data, SHA256: art.SHA256, Path: art.RemotePath()}, spec); err != nil {
		return err
	}
	c.UI.Info(fmt.Sprintf("Deployed %s %s (%s) in %s.", name, spec.ID, ref, time.Since(start).Round(100*time.Millisecond)))
	if local.Done != "" {
		env := []string{"TRELLIS_ENV=" + e.Name, "TRELLIS_SITE=" + name, "TRELLIS_COMMIT=" + art.Commit, "TRELLIS_RELEASE=" + spec.ID}
		if err := deploy.RunHook(ctx, local.Done, p.Root, env, &uiWriter{ui: c.UI}); err != nil {
			return fmt.Errorf("deployed, but deploy-hooks/done.sh failed: %w", err)
		}
	}
	return nil
}

func (c *ConvergeDeployCommand) Synopsis() string {
	return "Build a site locally and deploy it"
}

func (c *ConvergeDeployCommand) Help() string {
	return `Usage: trellis converge deploy [options] <environment> <site>

Builds the site locally from an exact commit of the clone holding its source
(never uncommitted changes): composer install with your own credentials, then
deploy-hooks/build.sh if present (e.g. theme assets). The packed release is
uploaded to the server, which links shared paths (.env, uploads), checks
the server's PHP platform, runs deploy-hooks/release.sh, switches current
atomically, updates the database and reloads PHP-FPM, then runs
deploy-hooks/after.sh. A failure before the switch leaves the site unchanged.
Finally deploy-hooks/done.sh runs locally (notifications, CDN purges).

Deploys log in as web with an access.deploy key and read no secrets, so CI
needs only a deploy key.

Per site in trellis.yml:

  deploy:
    keep: 5              # releases kept on servers
    update_db: true      # wp core update-db after the switch
    exclude: ["*.map", "web/app/themes/*/resources"]

Options:
  --ref           Branch, tag or commit, as your clone has it (default: the
                  site's branch, fetched from its remote; in CI (CI=true),
                  the checked-out commit)
` + connectionHelp
}

// --- rollback ----------------------------------------------------------------

type ConvergeRollbackCommand struct {
	UI      cli.Ui
	Version string
	flags   *flag.FlagSet
	conn    connection
}

func NewConvergeRollbackCommand(ui cli.Ui, version string) *ConvergeRollbackCommand {
	c := &ConvergeRollbackCommand{UI: ui, Version: version}
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.conn.register(c.flags)
	return c
}

func (c *ConvergeRollbackCommand) Run(args []string) int {
	args, err := parseArgs(c.flags, args)
	if err != nil {
		return 1
	}
	if len(args) != 2 {
		c.UI.Error("Usage: trellis converge rollback [options] <environment> <site>")
		return 1
	}
	env, name := args[0], args[1]
	p := openProject(c.UI)
	if p == nil {
		return 1
	}
	e := resolveEnv(c.UI, p, env)
	if e == nil {
		return 1
	}
	ctx, cancel := signalContext()
	defer cancel()
	if _, err := findSite(e, name); err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	s, srv, err := c.conn.deploySession(ctx, &uiWriter{ui: c.UI}, e)
	if err == nil {
		err = s.Rollback(ctx, srv, name, "php"+e.Server.PHP.Version+"-fpm.service")
	}
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	return 0
}

func (c *ConvergeRollbackCommand) Synopsis() string {
	return "Switch a site back to its previous release"
}

func (c *ConvergeRollbackCommand) Help() string {
	return `Usage: trellis converge rollback [options] <environment> <site>

Switches the site's current release to the one before it and reloads PHP-FPM.
Like deploy, it logs in as web and reads no secrets.

Options:
` + connectionHelp
}
