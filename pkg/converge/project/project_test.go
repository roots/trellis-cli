package project_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/roots/trellis-cli/pkg/converge/project"
	"github.com/roots/trellis-cli/pkg/converge/secrets"
)

// newProject writes a project whose production admins are alice (SSH key
// file) plus a CI age key, and returns it with both private keys.
func newProject(t *testing.T) (p *project.Project, aliceKey, ciKey string) {
	t.Helper()
	dir := t.TempDir()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sp, _ := ssh.NewPublicKey(pub)
	block, _ := ssh.MarshalPrivateKey(priv, "alice")
	aliceKey = filepath.Join(dir, "alice-private")
	ciIdentity, ciPub, err := secrets.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"alice-private":  string(pem.EncodeToMemory(block)),
		"keys/alice.pub": string(ssh.MarshalAuthorizedKey(sp)),
		"trellis.yml": `version: 1
sites:
  example.com: { repo: git@github.com:example/example.com.git, path: site }
access:
  admins: [file:keys/alice.pub]
  recipients: [` + ciPub + `]
environments:
  development:
    sites: { example.com: { domain: example.test } }
  production:
    nodes: [{ host: 203.0.113.10 }]
    sites: { example.com: { domain: example.com } }
`,
		"trellis.lock": "version: 1\nos: ubuntu-24.04\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p, err = project.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p, aliceKey, ciIdentity
}

func TestProductionSecretsAreEncryptedToAdminsAndRecipients(t *testing.T) {
	p, aliceKey, ciKey := newProject(t)

	doc, f, err := p.LoadSecrets("production", nil)
	if err != nil || f.Exists || !f.Encrypted {
		t.Fatalf("fresh production secrets: %+v, %v", f, err)
	}
	if _, err := doc.EnsureSite("example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.EnsureAdmin(); err != nil {
		t.Fatal(err)
	}
	b, _ := doc.Marshal()
	path, warnings, err := p.SaveSecrets("production", b)
	if err != nil || len(warnings) > 0 {
		t.Fatalf("save: %v %v", err, warnings)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasSuffix(path, "secrets/production.yml.age") || !secrets.IsEncrypted(data) {
		t.Fatalf("wrote %s:\n%s", path, data)
	}

	// Both alice (SSH key) and CI (age key) can compile.
	for name, keys := range map[string]secrets.Keys{
		"alice": secrets.KeySources{Files: []string{aliceKey}}.Keys(),
		"ci":    secrets.KeySources{AgeKey: ciKey}.Keys(),
	} {
		c, err := p.Compile("production", "test", keys)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if c.PlaintextSecrets != "" {
			t.Errorf("%s: reported plaintext", name)
		}
	}
	// Someone else can't.
	_, err = p.Compile("production", "test", secrets.KeySources{}.Keys())
	if err == nil || !strings.Contains(err.Error(), "production.yml.age") {
		t.Errorf("no keys: %v", err)
	}
}

func TestDevelopmentSecretsStayPlaintext(t *testing.T) {
	p, _, _ := newProject(t)
	path, _, err := p.SaveSecrets("development", []byte("a: b\n"))
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "a: b\n" || !strings.HasSuffix(path, "development.yml") {
		t.Errorf("%s = %q", path, data)
	}
	// No key is needed to read development secrets.
	if got, f, err := p.ReadSecrets("development", nil); err != nil || f.Encrypted || string(got) != "a: b\n" {
		t.Errorf("read development: %q %+v %v", got, f, err)
	}
}

func TestRekeyEncryptsPlaintextProductionSecrets(t *testing.T) {
	p, aliceKey, _ := newProject(t)
	plainPath := filepath.Join(p.Root, "secrets", "production.yml")
	_ = os.MkdirAll(filepath.Dir(plainPath), 0o755)
	if err := os.WriteFile(plainPath, []byte("mail:\n  password: pw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plain, f, err := p.ReadSecrets("production", nil)
	if err != nil || f.Encrypted {
		t.Fatalf("%+v %v", f, err)
	}
	path, warnings, err := p.SaveSecrets("production", plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plainPath); !os.IsNotExist(err) {
		t.Error("plaintext file was not removed")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "never committed") {
		t.Errorf("warnings = %v", warnings)
	}
	got, _, err := p.ReadSecrets("production", secrets.KeySources{Files: []string{aliceKey}}.Keys())
	if err != nil || string(got) != string(plain) {
		t.Errorf("read back %q, %v (from %s)", got, err, path)
	}
}

func TestRecipientsNeedLockedGitHubKeys(t *testing.T) {
	p, _, _ := newProject(t)
	yml := filepath.Join(p.Root, "trellis.yml")
	b, _ := os.ReadFile(yml)
	if err := os.WriteFile(yml, []byte(strings.Replace(string(b), "file:keys/alice.pub", "github:someone", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := project.Open(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.Recipients("production"); err == nil || !strings.Contains(err.Error(), "trellis converge lock") {
		t.Errorf("err = %v", err)
	}
}
