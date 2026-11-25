package manifest

import "io/fs"

// Constructors for each kind. They set Kind, Name and the default Phase so
// the compiler can't get the tagging wrong.

func NewPackage(name, version string) Resource {
	return Resource{Kind: KindPackage, Name: name, Phase: PhasePackages,
		Package: &Package{Name: name, Version: version}}
}

func NewFile(path, content string, mode fs.FileMode, owner, group string, notify ...Notify) Resource {
	return Resource{Kind: KindFile, Name: path, Phase: PhaseFiles, Notify: notify,
		File: &File{Path: path, Content: content, Mode: mode, Owner: owner, Group: group}}
}

// NewRootFile is a root:root file with the given mode.
func NewRootFile(path, content string, mode fs.FileMode, notify ...Notify) Resource {
	return NewFile(path, content, mode, "root", "root", notify...)
}

func NewAbsentFile(path string, notify ...Notify) Resource {
	return Resource{Kind: KindFile, Name: path, Phase: PhaseFiles, Notify: notify,
		File: &File{Path: path, Absent: true}}
}

func NewDownload(path, url string, mode fs.FileMode) Resource {
	return Resource{Kind: KindDownload, Name: path, Phase: PhaseFiles,
		Download: &Download{Path: path, URL: url, Mode: mode}}
}

func NewService(unit string, enabled *bool, active bool) Resource {
	return Resource{Kind: KindService, Name: unit, Phase: PhaseServices,
		Service: &Service{Unit: unit, Enabled: enabled, Active: active}}
}

func NewDatabase(name string, server *DBServer) Resource {
	return Resource{Kind: KindDatabase, Name: dbName(name, server), Phase: PhaseData,
		Database: &Database{Name: name, Server: server}}
}

func NewDatabaseUser(user, host, password string, databases []string, server *DBServer) Resource {
	return Resource{Kind: KindDatabaseUser, Name: dbName(user+"@"+host, server), Phase: PhaseData,
		DatabaseUser: &DatabaseUser{User: user, Host: host, Password: password, Databases: databases, Server: server}}
}

func NewCert(name string, provider CertProvider, domains []string, certPath, keyPath string, notify ...Notify) Resource {
	return Resource{Kind: KindCert, Name: name, Phase: PhaseCerts, Notify: notify,
		Cert: &Cert{Provider: provider, Domains: domains, CertPath: certPath, KeyPath: keyPath}}
}

// WithStdin gives an exec resource secret input.
func (r Resource) WithStdin(stdin string) Resource {
	if r.Exec != nil {
		x := *r.Exec
		x.Stdin = stdin
		r.Exec = &x
	}
	return r
}

func NewExec(name string, command []string, inputs string) Resource {
	return Resource{Kind: KindExec, Name: name, Phase: PhaseExec,
		Exec: &Exec{Command: command, Inputs: inputs}}
}

// InPhase returns r moved to phase p.
func (r Resource) InPhase(p Phase) Resource {
	r.Phase = p
	return r
}

// WithValidate sets a file's pre-write validation command.
func (r Resource) WithValidate(argv ...string) Resource {
	if r.File != nil {
		f := *r.File
		f.Validate = argv
		r.File = &f
	}
	return r
}

// WithSHA256 pins a download's content hash.
func (r Resource) WithSHA256(sum string) Resource {
	if r.Download != nil {
		d := *r.Download
		d.SHA256 = sum
		r.Download = &d
	}
	return r
}

// AsSensitive marks a file's content as secret.
func (r Resource) AsSensitive() Resource {
	if r.File != nil {
		f := *r.File
		f.Sensitive = true
		r.File = &f
	}
	return r
}

func dbName(name string, server *DBServer) string {
	if server == nil {
		return name
	}
	return server.Host + "/" + name
}

// RunCommand is a phase-end command notification.
func RunCommand(argv ...string) Notify { return Notify{Command: argv} }

// ReloadUnit is a deferred reload notification, gated on check.
func ReloadUnit(unit string, check ...string) Notify {
	return Notify{Unit: unit, Action: Reload, Check: check}
}

// RestartUnit is a deferred restart notification, gated on check.
func RestartUnit(unit string, check ...string) Notify {
	return Notify{Unit: unit, Action: Restart, Check: check}
}

// Ptr is a helper for optional fields.
func Ptr[T any](v T) *T { return &v }
