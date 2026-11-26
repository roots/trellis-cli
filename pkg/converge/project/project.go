// Package project is the local "shell" around the pure compiler: it finds
// trellis.yml, loads trellis.lock and the environment's secrets, and
// assembles a compile.Input. It also owns where secrets live and who they
// are encrypted to.
package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"filippo.io/age"

	"github.com/roots/trellis-cli/pkg/converge/compile"
	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/lock"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/secrets"
)

type Project struct {
	Root   string
	Config *config.Config
	Lock   *lock.Lock
}

// Open finds trellis.yml from dir upwards and loads it with its lockfile.
func Open(dir string) (*Project, error) {
	path, err := config.Find(dir)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	root := filepath.Dir(path)
	l, err := lock.Load(filepath.Join(root, lock.FileName))
	if err != nil {
		return nil, err
	}
	return &Project{Root: root, Config: cfg, Lock: l}, nil
}

func (p *Project) LockPath() string { return filepath.Join(p.Root, lock.FileName) }

// SecretsFile locates env's secrets file. Development defaults to
// plaintext (it only ever reaches a local VM); everything else is encrypted.
func (p *Project) SecretsFile(env string) (secrets.File, error) {
	return secrets.Locate(filepath.Join(p.Root, "secrets"), env, env == config.DevelopmentEnv)
}

// ReadSecrets returns env's plaintext secrets (empty if there is no file).
// keys is only used if the file is encrypted.
func (p *Project) ReadSecrets(env string, keys secrets.Keys) ([]byte, secrets.File, error) {
	f, err := p.SecretsFile(env)
	if err != nil || !f.Exists {
		return nil, f, err
	}
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, f, err
	}
	if !f.Encrypted {
		if secrets.IsEncrypted(data) {
			return nil, f, fmt.Errorf("%s is encrypted; rename it to %s", f.Path, strings.TrimSuffix(f.Path, secrets.PlaintextExt)+secrets.EncryptedExt)
		}
		return data, f, nil
	}
	plain, err := secrets.Decrypt(data, keys)
	if err != nil {
		return nil, f, fmt.Errorf("%s: %w", f.Path, err)
	}
	return plain, f, nil
}

// LoadSecrets parses env's secrets.
func (p *Project) LoadSecrets(env string, keys secrets.Keys) (*secrets.Doc, secrets.File, error) {
	plain, f, err := p.ReadSecrets(env, keys)
	if err != nil {
		return nil, f, err
	}
	doc, err := secrets.ParseDoc(plain)
	if err != nil {
		return nil, f, fmt.Errorf("%s: %w", f.Path, err)
	}
	return doc, f, nil
}

// Recipients are the public keys env's secrets are encrypted to: the
// environment's admins plus access.recipients. Warnings name keys that
// can't be age recipients (ECDSA, security keys).
func (p *Project) Recipients(env string) ([]age.Recipient, []string, error) {
	e, err := p.Config.Resolve(env)
	if err != nil {
		return nil, nil, err
	}
	refs := append(slices.Clone(e.Access.Admins), e.Access.Recipients...)
	keys, errs := compile.ResolveKeys(refs, p.Lock, os.DirFS(p.Root))
	if len(errs) > 0 {
		return nil, nil, errors.Join(errs...)
	}
	rs, skipped, err := secrets.ParseRecipients(keys)
	if err != nil {
		return nil, nil, err
	}
	var warnings []string
	for _, k := range skipped {
		warnings = append(warnings, fmt.Sprintf("%s can't decrypt secrets (age supports ed25519 and RSA SSH keys); add an age key for them to access.recipients", k))
	}
	if len(rs) == 0 {
		return nil, warnings, fmt.Errorf("%s has no admins or recipients who could decrypt its secrets; add access.admins or access.recipients to trellis.yml", env)
	}
	return rs, warnings, nil
}

// SaveSecrets writes plaintext secrets for env. Development secrets stay
// plaintext unless already encrypted; every other environment is always
// encrypted to its current recipients (replacing a plaintext file). It
// returns the path written and any warnings.
func (p *Project) SaveSecrets(env string, plaintext []byte) (string, []string, error) {
	if _, err := secrets.ParseDoc(plaintext); err != nil {
		return "", nil, err
	}
	f, err := p.SecretsFile(env)
	if err != nil {
		return "", nil, err
	}
	if !f.Encrypted && env == config.DevelopmentEnv {
		return f.Path, nil, secrets.WriteFile(f.Path, plaintext, 0o600)
	}
	rs, warnings, err := p.Recipients(env)
	if err != nil {
		return "", warnings, err
	}
	data, err := secrets.Encrypt(plaintext, rs)
	if err != nil {
		return "", warnings, err
	}
	path := filepath.Join(p.Root, "secrets", env+secrets.EncryptedExt)
	if err := secrets.WriteFile(path, data, 0o644); err != nil {
		return "", warnings, err
	}
	if !f.Encrypted && f.Exists {
		if err := os.Remove(f.Path); err != nil {
			return path, warnings, err
		}
		warnings = append(warnings, fmt.Sprintf("encrypted %s to %s and removed the plaintext file; make sure it was never committed", f.Path, path))
	}
	return path, warnings, nil
}

// Compiled is a compiled environment.
type Compiled struct {
	Env      *config.Environment
	Manifest *manifest.Manifest
	// PlaintextSecrets is set when a non-development environment's secrets
	// file is not encrypted.
	PlaintextSecrets string
	// SudoPassword is the admin user's sudo password, used to run the
	// agent (empty in development, where sudo is passwordless).
	SudoPassword string
}

// Compile resolves env and compiles it. keys decrypts the secrets file.
func (p *Project) Compile(env, version string, keys secrets.Keys) (*Compiled, error) {
	e, err := p.Config.Resolve(env)
	if err != nil {
		return nil, err
	}
	doc, f, err := p.LoadSecrets(env, keys)
	if err != nil {
		return nil, err
	}
	sec, err := doc.Map()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.Path, err)
	}
	ms, err := compile.Compile(compile.Input{
		Env:     e,
		Lock:    p.Lock,
		Secrets: sec,
		Project: os.DirFS(p.Root),
		Version: version,
	})
	if err != nil {
		return nil, fmt.Errorf("compile %s:\n%w", env, err)
	}
	c := &Compiled{Env: e, Manifest: ms}
	if env != config.DevelopmentEnv {
		c.SudoPassword, _ = sec.Lookup("admin_password")
	}
	if f.Exists && !f.Encrypted && env != config.DevelopmentEnv {
		c.PlaintextSecrets = f.Path
	}
	return c, nil
}

// KeyRefs lists every SSH key reference across all environments.
func (p *Project) KeyRefs() ([]string, error) {
	var refs []string
	for _, name := range p.Config.EnvironmentNames() {
		e, err := p.Config.Resolve(name)
		if err != nil {
			return nil, err
		}
		refs = append(refs, e.Access.Admins...)
		refs = append(refs, e.Access.Deploy...)
		refs = append(refs, e.Access.Recipients...)
	}
	return refs, nil
}
