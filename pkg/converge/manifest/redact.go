package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// RedactedPrefix marks a secret that was replaced by its hash.
const RedactedPrefix = "redacted:sha256:"

// IsRedacted reports whether v is a redacted secret.
func IsRedacted(v string) bool { return strings.HasPrefix(v, RedactedPrefix) }

// redact hashes v, salted with the resource ID so equal secrets in
// different resources don't share a hash. It is idempotent.
func redact(id ID, v string) string {
	if v == "" || IsRedacted(v) {
		return v
	}
	h := sha256.New()
	h.Write([]byte(id.String()))
	h.Write([]byte{0})
	h.Write([]byte(v))
	return RedactedPrefix + hex.EncodeToString(h.Sum(nil))
}

func (s *DBServer) redacted(id ID) *DBServer {
	if s == nil {
		return nil
	}
	c := *s
	c.Password = redact(id, c.Password)
	return &c
}

// Redacted returns a copy of r with every secret (sensitive file content,
// passwords) replaced by a salted hash. Digests are computed over this
// form, so a redacted resource has the same digest as the original: state
// on the server can record what was applied, and detect a changed secret,
// without keeping the secret itself.
func (r Resource) Redacted() Resource {
	switch {
	case r.File != nil && r.File.Sensitive:
		f := *r.File
		f.Content = redact(r.ID(), f.Content)
		r.File = &f
	case r.DatabaseUser != nil:
		u := *r.DatabaseUser
		u.Password = redact(r.ID(), u.Password)
		u.Server = u.Server.redacted(r.ID())
		r.DatabaseUser = &u
	case r.Exec != nil && r.Exec.Stdin != "":
		x := *r.Exec
		x.Stdin = redact(r.ID(), x.Stdin)
		r.Exec = &x
	case r.Database != nil && r.Database.Server != nil:
		d := *r.Database
		d.Server = d.Server.redacted(r.ID())
		r.Database = &d
	}
	return r
}

// Redacted returns a copy of m with every resource redacted.
func (m *Manifest) Redacted() *Manifest {
	out := New(m.Node, m.Roles)
	out.Warnings = m.Warnings
	out.Resources = make([]Resource, len(m.Resources))
	for i := range m.Resources {
		out.Resources[i] = m.Resources[i].Redacted()
	}
	out.reindex()
	return out
}
