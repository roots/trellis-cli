package secrets

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"filippo.io/age/armor"
	"golang.org/x/crypto/ssh"
)

const (
	EncryptedExt = ".yml.age"
	PlaintextExt = ".yml"
)

// File is an environment's secrets file.
type File struct {
	Path      string
	Encrypted bool
	Exists    bool
}

// Locate finds env's secrets file in dir: <env>.yml.age (encrypted) or
// <env>.yml (plaintext). When neither exists it returns where one would be
// created: plaintext if plaintextDefault (local development), else encrypted.
func Locate(dir, env string, plaintextDefault bool) (File, error) {
	enc := File{Path: filepath.Join(dir, env+EncryptedExt), Encrypted: true}
	plain := File{Path: filepath.Join(dir, env+PlaintextExt)}
	encOK, plainOK := exists(enc.Path), exists(plain.Path)
	switch {
	case encOK && plainOK:
		return File{}, fmt.Errorf("both %s and %s exist; remove one", enc.Path, plain.Path)
	case encOK:
		enc.Exists = true
		return enc, nil
	case plainOK:
		plain.Exists = true
		return plain, nil
	case plaintextDefault:
		return plain, nil
	}
	return enc, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsEncrypted reports whether data is an armored age file.
func IsEncrypted(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(data), []byte(armor.Header))
}

// Encrypt encrypts plaintext to recipients as an armored (text) age file.
func Encrypt(plaintext []byte, recipients []age.Recipient) ([]byte, error) {
	if len(recipients) == 0 {
		return nil, errors.New("no recipients to encrypt secrets to")
	}
	var buf bytes.Buffer
	aw := armor.NewWriter(&buf)
	w, err := age.Encrypt(aw, recipients...)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(plaintext); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	if err := aw.Close(); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// Decrypt decrypts an armored age file with the user's keys.
func Decrypt(ciphertext []byte, keys Keys) ([]byte, error) {
	ids, tried, err := keys()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no keys to decrypt secrets with (looked for %s); set TRELLIS_AGE_KEY or TRELLIS_AGE_KEY_FILE, or run `trellis converge secrets keygen`", strings.Join(tried, ", "))
	}
	r, err := age.Decrypt(armor.NewReader(bytes.NewReader(bytes.TrimSpace(ciphertext))), ids...)
	var noMatch *age.NoIdentityMatchError
	if errors.As(err, &noMatch) {
		return nil, fmt.Errorf("none of your keys (%s) can decrypt this file; someone with access must add your key to access.admins or access.recipients and run `trellis converge secrets rekey`", strings.Join(tried, ", "))
	}
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

// ParseRecipients parses public keys: age keys (age1…) and SSH keys
// (ssh-ed25519, ssh-rsa). Other SSH key types (ECDSA, security keys) can't
// be age recipients; they are returned in skipped. Duplicates are dropped.
func ParseRecipients(keys []string) (rs []age.Recipient, skipped []string, err error) {
	seen := map[string]bool{}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" || strings.HasPrefix(k, "#") || seen[k] {
			continue
		}
		seen[k] = true
		var r []age.Recipient
		switch {
		case strings.HasPrefix(k, "age1"):
			r, err = age.ParseRecipients(strings.NewReader(k))
		case strings.HasPrefix(k, "ssh-ed25519 "), strings.HasPrefix(k, "ssh-rsa "):
			var one age.Recipient
			one, err = agessh.ParseRecipient(k)
			r = []age.Recipient{one}
		default:
			skipped = append(skipped, keyLabel(k))
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("recipient %s: %w", keyLabel(k), err)
		}
		rs = append(rs, r...)
	}
	return rs, skipped, nil
}

// keyLabel shortens a public key for messages: its type and comment.
func keyLabel(k string) string {
	f := strings.Fields(k)
	switch {
	case len(f) >= 3:
		return f[0] + " " + f[len(f)-1]
	case len(f) > 0 && len(f[0]) > 24:
		return f[0][:24] + "…"
	case len(f) > 0:
		return f[0]
	}
	return k
}

// Keys returns the user's decryption identities and a description of
// where they looked.
type Keys func() (ids []age.Identity, tried []string, err error)

// KeySources are where the user's decryption keys come from, in order.
type KeySources struct {
	// AgeKey is identity text (from $TRELLIS_AGE_KEY; e.g. CI).
	AgeKey string
	// Files are age identity files or SSH private keys. Missing files are
	// skipped, except Required ones.
	Files    []string
	Required map[string]bool
	// Passphrase prompts for an encrypted SSH key's passphrase. It is only
	// called if that key is a recipient of the file being decrypted.
	Passphrase func(prompt string) (string, error)
}

// DefaultKeySources: $TRELLIS_AGE_KEY, $TRELLIS_AGE_KEY_FILE, the trellis
// age key (`secrets keygen`), then ~/.ssh/id_ed25519 and ~/.ssh/id_rsa.
func DefaultKeySources(configDir string, passphrase func(string) (string, error)) KeySources {
	s := KeySources{AgeKey: os.Getenv("TRELLIS_AGE_KEY"), Required: map[string]bool{}, Passphrase: passphrase}
	if f := os.Getenv("TRELLIS_AGE_KEY_FILE"); f != "" {
		s.Files = append(s.Files, f)
		s.Required[f] = true
	}
	s.Files = append(s.Files, filepath.Join(configDir, "age-key.txt"))
	if home, err := os.UserHomeDir(); err == nil {
		s.Files = append(s.Files, filepath.Join(home, ".ssh", "id_ed25519"), filepath.Join(home, ".ssh", "id_rsa"))
	}
	return s
}

// Load parses every available identity.
func (s KeySources) Load() ([]age.Identity, []string, error) {
	var ids []age.Identity
	var tried []string
	if s.AgeKey != "" {
		tried = append(tried, "$TRELLIS_AGE_KEY")
		got, err := age.ParseIdentities(strings.NewReader(s.AgeKey))
		if err != nil {
			return nil, tried, fmt.Errorf("TRELLIS_AGE_KEY: %w", err)
		}
		ids = append(ids, got...)
	}
	var missing []string
	for _, path := range s.Files {
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) && !s.Required[path] {
			missing = append(missing, path)
			continue
		}
		tried = append(tried, path)
		if err != nil {
			return nil, tried, err
		}
		got, err := s.parse(path, data)
		if err != nil {
			if s.Required[path] {
				return nil, tried, fmt.Errorf("%s: %w", path, err)
			}
			continue // e.g. an unsupported SSH key type
		}
		ids = append(ids, got...)
	}
	if len(ids) == 0 {
		// Nothing usable: say everywhere we looked.
		return nil, append(append([]string{"$TRELLIS_AGE_KEY"}, tried...), missing...), nil
	}
	return ids, tried, nil
}

func (s KeySources) parse(path string, data []byte) ([]age.Identity, error) {
	if bytes.Contains(data, []byte("AGE-SECRET-KEY-")) {
		return age.ParseIdentities(bytes.NewReader(data))
	}
	id, err := agessh.ParseIdentity(data)
	if err == nil {
		return []age.Identity{id}, nil
	}
	var missing *ssh.PassphraseMissingError
	if !errors.As(err, &missing) {
		return nil, err
	}
	pub := missing.PublicKey
	if pub == nil {
		b, err := os.ReadFile(path + ".pub")
		if err != nil {
			return nil, fmt.Errorf("encrypted key without a public key: %w", err)
		}
		if pub, _, _, _, err = ssh.ParseAuthorizedKey(b); err != nil {
			return nil, err
		}
	}
	prompt := func() ([]byte, error) {
		if s.Passphrase == nil {
			return nil, fmt.Errorf("%s is passphrase-protected and no prompt is available", path)
		}
		p, err := s.Passphrase("Passphrase for " + path)
		return []byte(p), err
	}
	enc, err := agessh.NewEncryptedSSHIdentity(pub, data, prompt)
	if err != nil {
		return nil, err
	}
	return []age.Identity{enc}, nil
}

// Keys returns a Keys that loads once, on first use, so commands that never
// decrypt never prompt or read private keys.
func (s KeySources) Keys() Keys {
	var (
		done  bool
		ids   []age.Identity
		tried []string
		err   error
	)
	return func() ([]age.Identity, []string, error) {
		if !done {
			ids, tried, err = s.Load()
			done = true
		}
		return ids, tried, err
	}
}

// NewIdentity generates an age key, returning the identity file content
// and its public key.
func NewIdentity() (identity, recipient string, err error) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return "", "", err
	}
	return fmt.Sprintf("# trellis secrets key\n# public key: %s\n%s\n", id.Recipient(), id), id.Recipient().String(), nil
}

// RecipientOf returns the public key of the identity in an age key file.
func RecipientOf(identityFile []byte) (string, error) {
	ids, err := age.ParseIdentities(bytes.NewReader(identityFile))
	if err != nil {
		return "", err
	}
	if x, ok := ids[0].(*age.X25519Identity); ok {
		return x.Recipient().String(), nil
	}
	if h, ok := ids[0].(*age.HybridIdentity); ok {
		return h.Recipient().String(), nil
	}
	return "", errors.New("unsupported identity type")
}

// WriteFile atomically writes data to path.
func WriteFile(path string, data []byte, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
