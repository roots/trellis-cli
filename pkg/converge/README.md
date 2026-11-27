# converge: Ansible-free provisioning (experimental)

`trellis converge` provisions servers from a single `trellis.yml` without
Ansible. It keeps Trellis's opinions (the outcome) and replaces how they are
applied.

```
trellis.yml + trellis.lock + secrets/<env>.yml + server/
        │  compile (local, pure, offline)
        ▼
the server's manifest ── every file's bytes, package, unit, DB, cert, notification
        │  ssh (one multiplexed connection)
        ▼
trellis-agent on the server: observe → plan → (approve) → apply → record generation
```

## Two binaries

`trellis` (the CLI) does everything that needs the project: config, secrets
and compiling. `trellis-agent` (`cmd/trellis-agent`) runs on servers and
links only `agent`, `engine`, `host`, `manifest` and `state`, which a test
enforces. It is ~7MB (~3MB gzipped) and stdlib-only.

The CLI embeds gzipped linux/amd64 and linux/arm64 agents
(`pkg/converge/agentbin`), so the agent on a server always matches the CLI that
sent the manifest. Agent builds are reproducible, so the same source gives
the same content-addressed path (`/usr/local/lib/trellis/agent-<sha>`) and
nothing is re-uploaded. Run `make agents` (`go generate ./pkg/converge/agentbin`)
before building the CLI; releases do this in a goreleaser hook. For
development, `--agent-binary` or `$TRELLIS_AGENT_DIR` override the embedded agents.

## Packages

| Package     | Role                                                                                     | Pure? |
|-------------|------------------------------------------------------------------------------------------|-------|
| `config`    | Strictly parse `trellis.yml` and resolve an environment (one merge rule, defaults, validation) | yes |
| `lock`      | `trellis.lock`: everything that would otherwise need the network (keys, digests, versions) | yes (fetchers injected) |
| `secrets`   | `secret:` references; age-encrypted secrets documents                                     | yes (keys injected) |
| `templates` | Built-in config templates, ejected overrides                                              | yes |
| `compile`   | `Input → []Manifest`. No network, clock or host access; same input ⇒ same hash            | yes |
| `manifest`  | The closed set of resource kinds and the canonical, hashable manifest                     | yes |
| `state`     | Observed host state and applied generations (plain data)                                  | yes |
| `engine`    | `Observe` (reads via `host.Host`), `Compute` (**pure** plan), `Apply` (writes via `host.Host`) | plan is pure |
| `host`      | The engine's only port to a machine. `Local` on the server; `hosttest.Fake` in tests      | – |
| `agent`     | Server-side request/stream protocol (JSON lines over stdin/stdout); `cmd/trellis-agent`   | – |
| `agentbin`  | Embedded linux agent executables (generated)                                              | – |
| `remote`    | SSH transport, agent upload, plan/apply/deploy sessions (`remote.Exec` injected)         | – |
| `project`   | Finds/loads a project and assembles a compile input                                       | – |

The CLI commands in `cmd/converge*.go` are a thin layer over these packages.

## Secrets

Each environment has one YAML document, `secrets/<env>.yml.age`, encrypted
with [age](https://age-encryption.org) (an armored text file). Development uses
plaintext `secrets/development.yml`.

- **Who can decrypt** comes from `trellis.yml`: the environment's
  `access.admins` (their ed25519/RSA SSH keys, the same ones that get server
  access) plus `access.recipients` (age keys, e.g. for CI). Onboarding someone
  means adding them to `access.admins` and having an existing admin run
  `trellis converge secrets rekey <env>`.
- **Decrypting** happens only locally, at compile time. Keys are tried from
  `$TRELLIS_AGE_KEY`, `$TRELLIS_AGE_KEY_FILE`, `~/.config/trellis/age-key.txt`
  (`secrets keygen`), `~/.ssh/id_ed25519` and `~/.ssh/id_rsa`. A passphrase is
  only asked for if that key can decrypt the file. age can't use SSH agents,
  so keys that live only in 1Password, Secretive or a hardware token need an
  age key (`secrets keygen`) in `access.recipients`.
- **Commands**: `init` (generate DB passwords and salts), `edit`, `set`
  (value from stdin or a prompt), `show` (also a git textconv driver for
  readable diffs), `rekey`, `keygen`. Edits keep comments and ordering.
- **On servers**, secrets exist only in the files that need them. Generation
  history and plans store salted hashes (see below).

## Testing without VMs

- **compile**: golden files (`compile/testdata/*.golden`) pin the full desired
  state of every example project. `go test ./pkg/converge/compile -update`, then
  review the diff, which is exactly what would change on servers.
- **engine**: `hosttest.Fake` simulates files, dpkg/apt (including
  `policy-rc.d`), systemctl, MariaDB and certbot in memory. Tests run the full
  loop and assert: convergence (a second plan is empty), constant-cost no-op
  runs, drift repair, rollback when `nginx -t` fails, pre-write validation
  (`visudo`), pruning, recovery from failed runs, download verification.
- **remote**: a fake `ssh` dispatches to the real agent against fake hosts, so
  the client, protocol and agent upload are tested end to end.

CI runs only these. The fake models what Trellis expects of the OS, not what
the OS does: bugs in how packages, systemd and sudo actually behave have only
been found on real VMs, and those runs are manual so far.

## Resource kinds

`package`, `file` (whole files only; optional pre-write `validate`, `absent`),
`download` (sha256 or OpenPGP fingerprints verified), `service`, `database`,
`database_user`, `cert` (self-signed in Go; Let's Encrypt via certbot webroot),
`exec` (re-runs only when its command, inputs, stdin or env change).

Users customize servers with files (`server/files/`, per-environment
overlays), packages, and `commands:`; a path table decides each file's phase
and effect. See [CUSTOMIZING.md](CUSTOMIZING.md).

Users and directories are not resource kinds: the compiler writes
`sysusers.d`/`tmpfiles.d` files and lets systemd converge them.

## Apply semantics

1. Phases in order: repos → packages (one apt transaction) → identity →
   files → certs → data → services → exec. Each phase ends with its command
   notifications (`apt-get update`, `systemd-sysusers`, `daemon-reload`, …).
2. Prunes (resources owned by the last generation but no longer desired), in
   reverse phase order. Databases are never dropped.
3. Deferred unit reloads/restarts, each gated by a check (`nginx -t`,
   `php-fpm -t`, `sshd -t`, `nft -c`, …). A failed check restores the files
   that triggered it.
4. Let's Encrypt issuance (nginx is already serving the ACME location).
5. The generation is recorded in `/var/lib/trellis`, **redacted**: sensitive
   file contents and passwords are replaced by salted hashes. Digests are
   computed over the redacted form, so changes (including a rotated secret)
   are still detected, but no secret survives in history. Plans sent back to
   the CLI are redacted the same way. Failed runs record only
   what completed, so retries redo exactly the unfinished work, and
   everything attempted stays owned for pruning.

`apply` sends the hash of the plan you reviewed. The agent refuses to apply
if the plan it computes differs.

## Operating systems

`os:` (in `server:`, or an environment's `server:`): `ubuntu-24.04` (default),
`ubuntu-26.04` or `debian-13`, listed once in `config.OSes`. Manifests record
it and the agent refuses to run on a server whose `/etc/os-release` says
otherwise, so upgrading a server's OS is a deliberate config change. Each has
been tested by hand on VMs (provision, no-op re-run, deploy). Signing keys are verified in Go
(OpenPGP fingerprints), so nothing depends on `gpg` being installed.

Adding an OS is one row in `config.OSes`, once nginx.org, packages.sury.org
and the MariaDB mirror publish for its codename. Debian 12 isn't supported:
it's past regular security support, and its Podman has no Quadlet.

## HTTPS

Sites serve HTTPS by default, with nothing to configure:

```yaml
sites:
  example.com: {}                 # automatic: Let's Encrypt (self-signed in development)
  internal.example.com:
    https: self-signed            # e.g. no public DNS
  legacy.example.com:
    https:                        # the site's own certificate (e.g. Cloudflare Origin CA)
      certificate: file:certs/legacy.pem
      key: secret:certs/legacy_key
  plain.example.com:
    https: false                  # plain HTTP
```

Let's Encrypt certificates cover the domain and its redirects, are issued
with certbot (webroot) at the end of an apply, and renew with certbot's own
timer. Until one is issued the site serves a self-signed placeholder, and a
failed issuance is retried by the next apply. A site's own certificate is
checked against its key when compiling.

## WordPress sandbox

PHP-FPM and WP-Cron run under systemd's sandboxing
(`systemd/php-sandbox.conf`, which `eject` can override), so code a plugin
lets an attacker run can't reach home directories (SSH keys, shell startup
files), use sudo, or write system directories. Files under `/srv/www` and
`/tmp` work as before, so no plugin needs changes. wp-cli run over SSH and
deploy steps run outside it.

A site's `.env` is readable by `web` only (PHP-FPM, WP-Cron and deploys run
as `web`), not by nginx.

## Updates

Converge installs missing packages; it doesn't upgrade installed ones.
unattended-upgrades does, daily (`apt-daily-upgrade.timer`, 6:00 to 7:00
server time), and `auto_updates:` decides what it installs:

- `all` (default): the distro's security and bugfix updates, and the
  bugfix and security releases of nginx (nginx.org stable), PHP (Sury; only
  the configured `php.version`, since the version is in the package names)
  and MariaDB (within the series in the repository URL).
- `distro`: the distro's default, its security updates only.
- `false`: nothing automatic.

`trellis converge upgrade <env>` runs unattended-upgrades now, in one
transaction, installing exactly what `auto_updates` allows (with `false`, what
`all` would). Run it after a server's first apply: fresh images are usually
far behind, and the nightly run installs one package at a time, so its first
batch would take minutes and only start the next morning. `apply` suggests it.

Upgrades restart their services when the packages say to (PHP-FPM and
MariaDB restart, nginx swaps its binary). Trellis's config files are kept
(`--force-confold`), so a package that ships new defaults isn't held back.
Packages pinned in `trellis.lock` are excluded. Plans report packages with
updates waiting and a pending reboot; reboots are left to you.

Major versions are always explicit: a new `php.version`, or a Trellis
release that moves MariaDB's series.

## One server per environment

An environment is one server (`host:`, optional `user:`/`port:`), running
nginx, PHP and MariaDB, or nginx and PHP with an external database
(`database:`, e.g. RDS), whose databases and users converge creates over the
network. Separate web and database servers aren't supported.

## Server access

SSH accepts keys only. `access.admins` keys log in as `admin` (passwordless
sudo; the login to use by hand) and as root, which is how the CLI provisions:
the same login on a fresh server (the key your provider installed) and after.
A sudo password would protect nothing that root login with the same keys
doesn't bypass, and providers' web consoles are the way back in if SSH is
lost. `user:` or `--user` provisions as another user with passwordless sudo
(e.g. `ubuntu` on images without root login). Deploys log in as `web`
(`access.deploy` keys, default: the admins).

## Editors, CI and agents

- `trellis converge schema` prints the JSON Schema for `trellis.yml`
  (generated from the config types; `config/trellis.schema.json` is the
  committed copy). With yaml-language-server (e.g. VS Code's YAML
  extension), the first line of `trellis.yml` enables completion,
  descriptions and inline errors:

  ```yaml
  # yaml-language-server: $schema=https://raw.githubusercontent.com/roots/trellis-cli/converge-engine/pkg/converge/config/trellis.schema.json
  ```
- `trellis converge validate [--json] [--config-only] [env...]` checks every
  environment offline (config, secrets, keys, `server/` files, templates) and
  exits non-zero on errors. `--json` gives one result per environment with
  errors as `path: message` strings, for CI and agents editing the config.

## Deploys

`trellis converge deploy <env> <site>` builds the release locally from the
site's `source` (its Bedrock directory in a git clone; default `../site`), at
an exact commit (the site's branch as it is on the remote, `--ref`, or in CI
the checked-out commit; then Composer and `deploy-hooks/build.sh`), uploads it,
and the agent activates it: link shared paths, check the PHP platform, run
`deploy-hooks/release.sh`, switch `current` atomically, update the
database, reload PHP-FPM, run `deploy-hooks/after.sh`, and keep 5 releases.
`rollback` switches back. Both run as `web` with an `access.deploy` key and
read no secrets, so a deploy key is all CI needs. See DEPLOY.md.

## Comparing with stock Trellis

`pkg/converge/fingerprint` compares the effective configuration of a stock
Trellis server and a converge one; `compare.yml` lists every intended
difference with its reason. See its README, and roots/trellis#1708 for the
Trellis-side findings.

## Not done yet

- `/etc/hosts` entries for site hostnames (needs the server's hostname at
  apply time, and coordination with cloud-init).
- apt package versions aren't pinned: third-party repositories (Sury, nginx.org,
  MariaDB) drop old versions, so exact pins would break fresh installs. Ubuntu
  can pin the whole archive with `apt_snapshot` instead.
- Major upgrades: a new MariaDB series (the repository URL changes, the
  package names don't) needs an upgrade path before `mariadbVersion` changes.
- Automated tests on real OSes (a VM matrix in CI).
- Not yet compared against Trellis: SSL/HSTS/HTTP2, the FastCGI cache,
  Redis/Memcached, multisite.
- Development VMs (`trellis vm`; `--lima` works with any Lima VM) and
  importing existing `group_vars`.
