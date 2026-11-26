package cmd

import (
	"bytes"
	"flag"
	"fmt"
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
	c.flags.StringVar(&c.ref, "ref", "", "Branch, tag or commit to deploy (default: the site's branch)")
	c.conn.register(c.flags)
	return c
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
	compiled := compileEnv(c.UI, p, env, c.Version)
	if compiled == nil {
		return 1
	}
	if err := c.deploy(p, compiled, name); err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	return 0
}

func (c *ConvergeDeployCommand) deploy(p *project.Project, compiled *project.Compiled, name string) error {
	ctx, cancel := signalContext()
	defer cancel()
	site, err := findSite(compiled.Env, name)
	if err != nil {
		return err
	}
	if site.LocalPath == "" {
		return fmt.Errorf("site %s has no local_path: deploys build from your local clone of the site", name)
	}
	ref := c.ref
	if ref == "" {
		ref = site.Branch
	}
	hooks, local, err := deploy.Hooks(p.Root)
	if err != nil {
		return err
	}

	start := time.Now()
	c.UI.Info(fmt.Sprintf("Building %s at %s…", name, ref))
	var buildOut bytes.Buffer
	art, err := deploy.Build(ctx, deploy.BuildOptions{
		Repo:    filepath.Join(p.Root, site.LocalPath),
		Ref:     ref,
		Subtree: site.Path,
		Hook:    local.Build,
		Env:     []string{"TRELLIS_ENV=" + compiled.Env.Name, "TRELLIS_SITE=" + name},
		Exclude: site.Deploy.Exclude,
		Out:     &buildOut,
	})
	if err != nil {
		if buildOut.Len() > 0 {
			c.UI.Error(buildOut.String())
		}
		return err
	}
	c.UI.Info(fmt.Sprintf("Built %s: %d files, %.1f MB (%s)\n", art.Commit[:7], art.Files, float64(len(art.Data))/1e6, time.Since(start).Round(100*time.Millisecond)))

	spec := deploy.Spec(compiled.Env, site, art, hooks, time.Now())
	s, srv, err := c.conn.session(ctx, &uiWriter{ui: c.UI}, compiled)
	if err != nil {
		return err
	}
	if err := s.Deploy(ctx, srv, remote.Artifact{Data: art.Data, SHA256: art.SHA256, Path: art.RemotePath()}, spec); err != nil {
		return err
	}
	c.UI.Info(fmt.Sprintf("Deployed %s %s (%s) in %s.", name, spec.ID, ref, time.Since(start).Round(100*time.Millisecond)))
	if local.Done != "" {
		env := []string{"TRELLIS_ENV=" + compiled.Env.Name, "TRELLIS_SITE=" + name, "TRELLIS_COMMIT=" + art.Commit, "TRELLIS_RELEASE=" + spec.ID}
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

Builds the site locally from an exact commit of its local_path repository
(never uncommitted changes): composer install with your own credentials, then
deploy-hooks/build.sh if present (e.g. theme assets). The packed release is
uploaded to the server, which links shared paths (.env, uploads), checks
the server's PHP platform, runs deploy-hooks/release.sh, switches current
atomically, updates the database and reloads PHP-FPM, then runs
deploy-hooks/after.sh. A failure before the switch leaves the site unchanged.
Finally deploy-hooks/done.sh runs locally (notifications, CDN purges).

Per site in trellis.yml:

  deploy:
    keep: 5              # releases kept on servers
    update_db: true      # wp core update-db after the switch
    exclude: ["*.map", "web/app/themes/*/resources"]

Options:
  --ref           Branch, tag or commit (default: the site's branch)
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
	compiled := compileEnv(c.UI, p, env, c.Version)
	if compiled == nil {
		return 1
	}
	ctx, cancel := signalContext()
	defer cancel()
	if _, err := findSite(compiled.Env, name); err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	s, srv, err := c.conn.session(ctx, &uiWriter{ui: c.UI}, compiled)
	if err == nil {
		err = s.Rollback(ctx, srv, name, "php"+compiled.Env.Server.PHP.Version+"-fpm.service")
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

Options:
` + connectionHelp
}
