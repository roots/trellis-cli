package compile

import (
	"maps"

	m "github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/templates"
)

// mariadbVersion is the MariaDB series. Its repository URL is per series
// but the package names aren't, so changing it makes unattended-upgrades
// (auto_updates: all) do a major upgrade on existing servers the next
// morning. Don't change it without an upgrade path.
const mariadbVersion = "11.8"

// db compiles the MariaDB server and every site's database and user.
func (b *builder) db() {
	b.repo(aptRepo{
		name: "mariadb", uri: "https://mirror.rackspace.com/mariadb/repo/" + mariadbVersion + "/" + b.distro,
		suite: b.codename, components: "main",
		keyURL: "https://mariadb.org/mariadb_release_signing_key.asc", keyFile: "mariadb.asc",
		fingerprints: []string{mariadbFingerprint}, origin: "MariaDB",
	})
	b.pkg("mariadb-server", "mariadb-client")

	settings := maps.Clone(b.env.Server.MariaDB)
	if settings == nil {
		settings = map[string]string{}
	}

	b.add(
		m.NewRootFile("/etc/mysql/mariadb.conf.d/90-trellis.cnf", b.render("mariadb/server.cnf", map[string]any{
			"BindAddress": "127.0.0.1", "Settings": templates.Sorted(settings),
		}), 0o644, mariadbRestart),
		m.NewService("mariadb.service", enabled, true),
	)
	b.siteDatabases(nil, "localhost")
}

// siteDatabases declares each site's database and user on server (nil =
// the local server).
func (b *builder) siteDatabases(server *m.DBServer, clientHost string) {
	for _, s := range b.env.Sites {
		pw := b.required("sites", s.Name, "db_password")
		b.add(
			m.NewDatabase(s.DBName(), server),
			m.NewDatabaseUser(s.DBUser(), clientHost, pw, []string{s.DBName()}, server),
		)
	}
}

// externalDatabases is compiled when the database is
// external: databases are created over the network with admin credentials,
// or left to the user (with a warning) when none are given.
func (b *builder) externalDatabases() {
	db := b.env.Database
	if db == nil {
		return
	}
	if db.Admin == nil {
		b.warnf("database %s is external without admin credentials; create each site's database and user yourself", db.Host)
		return
	}
	server := &m.DBServer{
		Host:     db.Host,
		Port:     db.Port,
		User:     db.Admin.User,
		Password: b.secret(db.Admin.Password, "database.admin.password"),
		TLS:      db.TLS == "required",
	}
	b.siteDatabases(server, "%")
}
