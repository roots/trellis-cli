package manifest

import (
	"fmt"
	"io"
	"strings"
)

// Describe is a one-line human summary of a resource's desired state.
func (r *Resource) Describe() string {
	switch r.Kind {
	case KindPackage:
		if r.Package.Version != "" {
			return r.Package.Name + "=" + r.Package.Version
		}
		return r.Package.Name
	case KindFile:
		if r.File.Absent {
			return r.File.Path + " (absent)"
		}
		return fmt.Sprintf("%s %04o %s:%s", r.File.Path, r.File.Mode, r.File.Owner, r.File.Group)
	case KindDownload:
		return r.Download.Path + " ← " + r.Download.URL
	case KindService:
		s := r.Service.Unit
		if r.Service.Enabled != nil && *r.Service.Enabled {
			s += " enabled"
		}
		if r.Service.Active {
			s += " active"
		}
		return s
	case KindDatabase:
		return r.Name
	case KindDatabaseUser:
		return r.Name + " → " + strings.Join(r.DatabaseUser.Databases, ",")
	case KindCert:
		return fmt.Sprintf("%s %s [%s]", r.Name, r.Cert.Provider, strings.Join(r.Cert.Domains, " "))
	case KindExec:
		return r.Name + ": " + strings.Join(r.Exec.Command, " ")
	}
	return r.ID().String()
}

// Render writes a readable, deterministic listing of the manifest. File
// contents are included unless sensitive. It is used for golden tests and
// `trellis converge compile`.
func (m *Manifest) Render(w io.Writer, withContent bool) {
	fmt.Fprintf(w, "# server %s (%s, %s)\n# hash %s\n", m.Node, strings.Join(m.Roles, " + "), m.OS, m.Hash())
	for _, warn := range m.Warnings {
		fmt.Fprintf(w, "# warning: %s\n", warn)
	}
	phase := Phase(-1)
	for i := range m.Resources {
		r := &m.Resources[i]
		if r.Phase != phase {
			phase = r.Phase
			fmt.Fprintf(w, "\n== %s\n", phase)
		}
		fmt.Fprintf(w, "%-13s %s\n", r.Kind, r.Describe())
		for _, n := range r.Notify {
			if n.Unit != "" {
				fmt.Fprintf(w, "              ↳ %s %s", n.Action, n.Unit)
			} else {
				fmt.Fprintf(w, "              ↳ run %s", strings.Join(n.Command, " "))
			}
			if len(n.Check) > 0 {
				fmt.Fprintf(w, " (check: %s)", strings.Join(n.Check, " "))
			}
			_, _ = fmt.Fprintln(w)
		}
		if withContent && r.Kind == KindFile && !r.File.Absent {
			if r.File.Sensitive {
				fmt.Fprintf(w, "    │ <sensitive: %d bytes>\n", len(r.File.Content))
				continue
			}
			for _, line := range strings.Split(strings.TrimRight(r.File.Content, "\n"), "\n") {
				fmt.Fprintf(w, "    │ %s\n", line)
			}
		}
	}
}
