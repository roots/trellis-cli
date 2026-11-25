package compile

import (
	"maps"

	"github.com/roots/trellis-cli/pkg/converge/config"
	m "github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/templates"
)

const mariadbVersion = "11.8"

// db compiles the MariaDB server and every site's database and user.
func (b *builder) db() {
	b.repo(aptRepo{
		name: "mariadb", uri: "https://mirror.rackspace.com/mariadb/repo/" + mariadbVersion + "/ubuntu",
		suite: b.codename, components: "main",
		keyURL: "https://mariadb.org/mariadb_release_signing_key.asc", keyFile: "mariadb.asc",
	})
	b.pkg("mariadb-server", "mariadb-client")

	bind := "127.0.0.1"
	clientHost := "localhost"
	if !b.isWeb() {
		bind = b.node.PrivateIP
		for _, n := range b.env.Nodes {
			if n.HasRole(config.RoleWeb) {
				clientHost = n.Address()
			}
		}
	}
	settings := maps.Clone(b.env.Server.MariaDB)
	if settings == nil {
		settings = map[string]string{}
	}
	maps.Copy(settings, b.node.MariaDB)

	b.add(
		m.NewRootFile("/etc/mysql/mariadb.conf.d/90-trellis.cnf", b.render("mariadb/server.cnf", map[string]any{
			"BindAddress": bind, "Settings": templates.Sorted(settings),
		}), 0o644, mariadbRestart),
		m.NewService("mariadb.service", enabled, true),
	)
	b.siteDatabases(nil, clientHost)
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

// externalDatabases is compiled on the web node when the database is
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
