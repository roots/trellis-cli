package secrets

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestRefs(t *testing.T) {
	m := Map{"mail": map[string]any{"password": "pw"}, "n": 5}
	if v, err := Resolve(m, "secret:mail/password"); err != nil || v != "pw" {
		t.Errorf("Resolve = %q, %v", v, err)
	}
	if v, _ := Resolve(m, "literal"); v != "literal" {
		t.Errorf("literal = %q", v)
	}
	if _, err := Resolve(m, "secret:missing"); err == nil {
		t.Error("missing secret must error")
	}
	if v, ok := m.Lookup("n"); !ok || v != "5" {
		t.Errorf("number = %q", v)
	}
}

func TestDocEditsPreserveComments(t *testing.T) {
	doc, err := ParseDoc([]byte("# team secrets\nmail:\n  password: old # rotated 2026\n"))
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := doc.Set([]string{"mail", "password"}, "new", false); err != nil || changed {
		t.Errorf("no-overwrite Set changed=%v err=%v", changed, err)
	}
	if changed, _ := doc.Set([]string{"mail", "password"}, "new", true); !changed {
		t.Error("overwrite Set did not change")
	}
	if changed, _ := doc.Set([]string{"stripe", "key"}, "0123", true); !changed {
		t.Error("nested Set did not change")
	}
	if _, err := doc.Set([]string{"mail"}, "x", true); err == nil {
		t.Error("replacing a section with a value must error")
	}
	if _, err := doc.Set([]string{"mail", "password", "x"}, "x", true); err == nil {
		t.Error("descending into a value must error")
	}
	b, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# team secrets", "# rotated 2026", "password: new"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %q in:\n%s", want, b)
		}
	}
	again, err := ParseDoc(b)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := again.Map()
	if v, _ := m.Lookup("stripe", "key"); v != "0123" {
		t.Errorf("numeric-looking string round-tripped as %q", v)
	}
}

func TestEnsureSiteIsAdditive(t *testing.T) {
	doc, _ := ParseDoc([]byte("sites:\n  a.com:\n    db_password: keep\n"))
	added, err := doc.EnsureSite("a.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != len(SaltKeys) {
		t.Errorf("added %v", added)
	}
	if v, _ := doc.Get("sites", "a.com", "db_password"); v != "keep" {
		t.Error("existing password replaced")
	}
	if v, _ := doc.Get("sites", "a.com", "salts", "nonce_salt"); len(v) != 64 {
		t.Errorf("salt = %q", v)
	}
	if again, _ := doc.EnsureSite("a.com"); len(again) != 0 {
		t.Errorf("second EnsureSite added %v", again)
	}
}

func TestParseDocRejectsNonMapping(t *testing.T) {
	if _, err := ParseDoc([]byte("- a\n- b\n")); err == nil {
		t.Error("a list must be rejected")
	}
	if d, err := ParseDoc(nil); err != nil || d == nil {
		t.Errorf("empty input: %v", err)
	}
}

type sshKey struct {
	pub     string // authorized_keys line
	path    string // private key file
	encPath string // passphrase-protected copy
}

func newSSHKey(t *testing.T, dir, name string) sshKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp, _ := ssh.NewPublicKey(pub)
	k := sshKey{pub: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sp))) + " " + name, path: filepath.Join(dir, name), encPath: filepath.Join(dir, name+"-enc")}
	block, _ := ssh.MarshalPrivateKey(priv, name)
	encBlock, _ := ssh.MarshalPrivateKeyWithPassphrase(priv, name, []byte("hunter2"))
	if err := os.WriteFile(k.path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(k.encPath, pem.EncodeToMemory(encBlock), 0o600); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestEncryptToAgeAndSSHRecipients(t *testing.T) {
	dir := t.TempDir()
	alice, bob := newSSHKey(t, dir, "alice"), newSSHKey(t, dir, "bob")
	ciIdentity, ciPub, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rs, skipped, err := ParseRecipients([]string{alice.pub, ciPub, alice.pub})
	if err != nil || len(rs) != 2 || len(skipped) != 0 {
		t.Fatalf("recipients = %d, skipped %v, err %v", len(rs), skipped, err)
	}
	plain := []byte("mail:\n  password: s3cret\n")
	ct, err := Encrypt(plain, rs)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncrypted(ct) || strings.Contains(string(ct), "s3cret") {
		t.Fatalf("not encrypted:\n%s", ct)
	}

	decrypt := func(s KeySources) (string, error) {
		out, err := Decrypt(ct, s.Keys())
		return string(out), err
	}
	if got, err := decrypt(KeySources{AgeKey: ciIdentity}); err != nil || got != string(plain) {
		t.Errorf("CI key: %q, %v", got, err)
	}
	if got, err := decrypt(KeySources{Files: []string{bob.path, alice.path}}); err != nil || got != string(plain) {
		t.Errorf("alice's SSH key: %q, %v", got, err)
	}

	// A passphrase-protected key prompts, but only when it is a recipient.
	prompts := 0
	ask := func(string) (string, error) { prompts++; return "hunter2", nil }
	if got, err := decrypt(KeySources{Files: []string{bob.encPath, alice.encPath}, Passphrase: ask}); err != nil || got != string(plain) {
		t.Errorf("encrypted SSH key: %q, %v", got, err)
	}
	if prompts != 1 {
		t.Errorf("prompted %d times, want 1 (only for alice)", prompts)
	}

	_, err = decrypt(KeySources{Files: []string{bob.path}})
	if err == nil || !strings.Contains(err.Error(), "access.admins") {
		t.Errorf("non-recipient error = %v", err)
	}
	_, err = decrypt(KeySources{Files: []string{filepath.Join(dir, "missing")}})
	if err == nil || !strings.Contains(err.Error(), "no keys") {
		t.Errorf("no-keys error = %v", err)
	}
}

func TestUnsupportedSSHKeysAreSkipped(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	sp, _ := ssh.NewPublicKey(&priv.PublicKey)
	ecdsaKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sp))) + " carol@laptop"
	rs, skipped, err := ParseRecipients([]string{ecdsaKey, "sk-ssh-ed25519@openssh.com AAAA dave"})
	if err != nil || len(rs) != 0 || len(skipped) != 2 || !strings.Contains(skipped[0], "carol@laptop") {
		t.Errorf("rs=%d skipped=%v err=%v", len(rs), skipped, err)
	}
	if _, _, err := ParseRecipients([]string{"ssh-ed25519 notbase64 x"}); err == nil {
		t.Error("an invalid key must error")
	}
}

func TestLocate(t *testing.T) {
	dir := t.TempDir()
	if f, _ := Locate(dir, "production", false); !f.Encrypted || f.Exists || filepath.Base(f.Path) != "production.yml.age" {
		t.Errorf("default production = %+v", f)
	}
	if f, _ := Locate(dir, "development", true); f.Encrypted || filepath.Base(f.Path) != "development.yml" {
		t.Errorf("default development = %+v", f)
	}
	_ = os.WriteFile(filepath.Join(dir, "production.yml"), nil, 0o600)
	if f, _ := Locate(dir, "production", false); f.Encrypted || !f.Exists {
		t.Errorf("plaintext production = %+v", f)
	}
	_ = os.WriteFile(filepath.Join(dir, "production.yml.age"), nil, 0o644)
	if _, err := Locate(dir, "production", false); err == nil {
		t.Error("both files present must error")
	}
}
