package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/roots/trellis-cli/pkg/converge/config"
	m "github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/templates"
)

// services compiles Podman Quadlet containers (rung 4: other software).
func (b *builder) services() {
	b.externalDatabases()
	svcs := b.env.Server.Services
	if len(svcs) == 0 {
		return
	}
	b.pkg("podman")
	for _, name := range sortedKeys(svcs) {
		svc := svcs[name]
		image := svc.Image
		if digest, ok := b.in.Lock.Images[svc.Image]; ok {
			image = svc.Image + "@" + digest
		} else {
			b.warnf("service %s: image %s is not pinned to a digest in trellis.lock", name, svc.Image)
		}
		unit := name + ".service"
		envFile := "/etc/trellis/services/" + name + ".env"
		data := map[string]any{"Name": name, "Image": image, "Service": svc, "EnvFile": envFile}
		b.add(
			m.NewRootFile("/etc/containers/systemd/"+name+".container", b.render("podman/service.container", data), 0o644,
				daemonReload, m.RestartUnit(unit)),
			// Quadlet units are enabled by their generator, not systemctl.
			m.NewService(unit, nil, true),
		)
		if len(svc.Env) > 0 {
			var sb strings.Builder
			for _, kv := range templates.Sorted(svc.Env) {
				sb.WriteString(kv.Key + "=" + b.secret(kv.Value, "services."+name+".env."+kv.Key) + "\n")
			}
			b.add(m.NewRootFile(envFile, sb.String(), 0o600, m.RestartUnit(unit)).AsSensitive())
		}
	}
}

// userExec compiles exec hooks (the escape hatch). Each re-runs only when
// the hash of its command and input files changes.
func (b *builder) userExec() {
	for _, x := range b.env.Server.Exec {
		h := sha256.New()
		h.Write([]byte(x.Run))
		for _, in := range x.Inputs {
			if err := hashTree(h, b.in.Project, strings.TrimSuffix(in, "/")); err != nil {
				b.errorf("exec %s: input %s: %v", x.Name, in, err)
			}
		}
		b.add(m.NewExec("user:"+x.Name, []string{"/bin/sh", "-c", x.Run}, hex.EncodeToString(h.Sum(nil))))
	}
}

// hashTree writes every file under root (sorted, with its path) into h.
func hashTree(h interface{ Write([]byte) (int, error) }, fsys fs.FS, root string) error {
	if fsys == nil {
		return errors.New("no project directory")
	}
	return fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		_, _ = h.Write([]byte(p + "\x00"))
		_, _ = h.Write(data)
		return nil
	})
}

// dropIns installs user files from server/ (rung 2). Paths are fixed by
// convention; anything unrecognized is an error so typos don't silently
// do nothing. Files ending in .tmpl are rendered with a typed context.
func (b *builder) dropIns() {
	if b.in.Project == nil {
		return
	}
	// Exec inputs may live under server/ (e.g. server/fonts/); they are
	// consumed by their hook, not installed.
	var execInputs []string
	for _, x := range b.env.Server.Exec {
		for _, in := range x.Inputs {
			execInputs = append(execInputs, strings.TrimSuffix(in, "/"))
		}
	}
	err := fs.WalkDir(b.in.Project, "server", func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && p == "server" {
			return fs.SkipAll
		}
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "server/")
		if d.IsDir() {
			if rel == "ejected" || slices.Contains(execInputs, p) {
				return fs.SkipDir
			}
			return nil
		}
		if slices.Contains(execInputs, p) {
			return nil
		}
		if strings.HasPrefix(path.Base(rel), ".") {
			return nil
		}
		b.dropIn(p, rel)
		return nil
	})
	if err != nil {
		b.errorf("server/: %v", err)
	}
}

func (b *builder) dropIn(full, rel string) {
	parts := strings.Split(rel, "/")
	name := strings.TrimSuffix(parts[len(parts)-1], ".tmpl")
	var site *config.Site

	var dest string
	var notify []m.Notify
	switch {
	case len(parts) == 3 && parts[0] == "nginx" && parts[1] == "http.d":
		if !b.isWeb() {
			return
		}
		dest, notify = "/etc/nginx/conf.d/"+name, []m.Notify{nginxReload}
	case len(parts) == 4 && parts[0] == "nginx" && parts[1] == "sites":
		if !b.isWeb() {
			return
		}
		site = b.findSite(parts[2])
		if site == nil {
			return // site isn't deployed to this environment
		}
		dest, notify = "/etc/nginx/includes.d/"+parts[2]+"/"+name, []m.Notify{nginxReload}
	case len(parts) == 3 && parts[0] == "php" && parts[1] == "conf.d":
		if !b.isWeb() {
			return
		}
		dest, notify = "/etc/php/"+b.phpVersion()+"/fpm/conf.d/"+name, []m.Notify{b.phpReload()}
	case len(parts) == 3 && parts[0] == "mariadb" && parts[1] == "conf.d":
		if !b.isDB() {
			return
		}
		dest, notify = "/etc/mysql/mariadb.conf.d/"+name, []m.Notify{mariadbRestart}
	case len(parts) == 3 && parts[0] == "systemd" && strings.HasSuffix(parts[1], ".d"):
		unit := strings.TrimSuffix(parts[1], ".d")
		dest, notify = "/etc/systemd/system/"+parts[1]+"/"+name, []m.Notify{daemonReload, m.RestartUnit(unit)}
	default:
		b.errorf("server/%s: unrecognized drop-in location (see `trellis converge help drop-ins`)", rel)
		return
	}

	data, err := fs.ReadFile(b.in.Project, full)
	if err != nil {
		b.errorf("server/%s: %v", rel, err)
		return
	}
	content := string(data)
	if strings.HasSuffix(rel, ".tmpl") {
		t, err := templates.Parse(rel, content)
		if err != nil {
			b.errs = append(b.errs, err)
			return
		}
		content, err = templates.Execute(t, map[string]any{
			"Env": b.env.Name, "Node": b.node, "Server": b.env.Server, "Site": site,
		})
		if err != nil {
			b.errs = append(b.errs, err)
			return
		}
	}
	b.add(m.NewRootFile(dest, content, 0o644, notify...))
}

func (b *builder) findSite(name string) *config.Site {
	for i := range b.env.Sites {
		if b.env.Sites[i].Name == name {
			return &b.env.Sites[i]
		}
	}
	return nil
}
