// Package secrets holds an environment's secrets: a YAML document stored as
// secrets/<env>.yml.age, encrypted with age to the environment's admins'
// SSH keys (from trellis.yml) plus any extra age recipients.
//
// trellis.yml refers to values with `secret:<path>` (e.g.
// `secret:mail/password`); generated per-site secrets (database password,
// WordPress salts) live under sites/<name>/.
package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

const RefPrefix = "secret:"

// Store looks up secret values by path.
type Store interface {
	Lookup(path ...string) (string, bool)
}

// Map is a decoded secrets document.
type Map map[string]any

func (m Map) Lookup(path ...string) (string, bool) {
	var cur any = map[string]any(m)
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		if cur, ok = mm[p]; !ok {
			return "", false
		}
	}
	switch v := cur.(type) {
	case string:
		return v, true
	case int, int64, float64, bool:
		return fmt.Sprint(v), true
	}
	return "", false
}

// IsRef reports whether v is a secret reference.
func IsRef(v string) bool { return strings.HasPrefix(v, RefPrefix) }

// RefPath splits "secret:a/b" (or "a/b") into ["a", "b"].
func RefPath(v string) []string {
	return strings.Split(strings.TrimPrefix(v, RefPrefix), "/")
}

// Resolve returns v, or the secret it references.
func Resolve(s Store, v string) (string, error) {
	if !IsRef(v) {
		return v, nil
	}
	val, ok := s.Lookup(RefPath(v)...)
	if !ok {
		return "", fmt.Errorf("secret %q is not defined in the secrets file", strings.TrimPrefix(v, RefPrefix))
	}
	return val, nil
}

// SaltKeys are the WordPress salts generated per site.
var SaltKeys = []string{
	"auth_key", "secure_auth_key", "logged_in_key", "nonce_key",
	"auth_salt", "secure_auth_salt", "logged_in_salt", "nonce_salt",
}

// Random returns n random URL-safe characters.
func Random(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b)[:n], nil
}
