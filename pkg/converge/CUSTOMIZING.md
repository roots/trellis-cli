# Customizing servers

Trellis configures nginx, PHP, MariaDB and the base system from
`trellis.yml`. Everything else is customized with a few primitives, the same
resource kinds the engine plans, diffs, prunes and rolls back. There's no
task language. Anything that can't be expressed with them goes through an
escape hatch: a command, or leaving that part of the server unmanaged.

## The primitives

| Primitive | Where | Lifecycle |
|---|---|---|
| **File** | `server/files/<path>` → `/<path>` on the server | Planned, diffed, pruned when removed |
| **Package** | `packages:` in `trellis.yml` | Installed; pruned when removed |
| **Service** | Inferred from a unit file's `[Install]` section | Enabled and started; stopped and disabled when the file is removed |
| **Command** | `commands:` in `trellis.yml` | Runs once, re-runs when it or a watched file changes, never undone |

Plus two ways to vary them:

- **Per-environment overlays:** `server/env/<env>/files/<path>` applies only to
  that environment, and replaces `server/files/<path>` there.
- **Templates:** a file ending in `.tmpl` is rendered (Go templates). It sees
  `.Env`, `.Host`, `.Server` (server settings), `.Sites`, `.Site` (for
  `/etc/nginx/includes.d/<site>/`) and `{{ secret "path" }}`. A file that uses
  a secret is *sensitive*: mode 0600 (0640 with the service's group where one
  reads it), and its content stays out of plans and server history.

## Rules

These keep the surface small. A request that needs a rule broken is a design
discussion, not a new option.

1. **Closed set of resource kinds.** New kinds are rare Trellis changes, never
   user extensions.
2. **Whole files, one owner per path.** No line editing. If the distro owns a
   file, use its `.d` directory. If Trellis owns it, use its drop-in directory
   or eject the template (`trellis converge eject`), an explicit, tracked
   takeover.
3. **Effects come from paths.** A file's phase and what a change to it does
   come from the table below. There's no user-defined ordering: phases are
   fixed and commands run last.
4. **No logic in config.** No `when:` or loops. Overlays and templates are the
   only conditionals.
5. **Customizations are data in the project.** Compiling stays offline and
   deterministic.
6. **Options exist only for Trellis's own behaviour.** An option in
   `trellis.yml` is a Trellis opinion that many sites tune (PHP, nginx,
   MariaDB). **Don't wrap a tool's native config in Trellis YAML:** for
   anything else, write the tool's own file.
7. **Trellis manages only what it owns.** Files it doesn't own are never
   touched, so hand changes, cloud-init and other tools can coexist with it.

## What a path does

The first matching rule applies. Paths that match nothing are installed in
the files phase with no effect (mode 0755 if the project file is
executable, else 0644); a command with `on:` can react to them.

| Path | Phase | On change |
|---|---|---|
| `/etc/apt/{keyrings,sources.list.d,preferences.d,apt.conf.d,trusted.gpg.d}/*`, `/usr/share/keyrings/*` | repos (before packages) | `apt-get update` |
| `/etc/nginx/**` | files | `nginx -t`, then reload (rolled back if the check fails) |
| `/etc/nginx/includes.d/<site>/*` | files | as above; skipped in environments without that site |
| `/etc/php/<version>/{fpm,mods-available}/**` | files | `php-fpm -t`, then reload (the version must be the server's) |
| `/etc/mysql/**` | files | restart MariaDB (if it runs on the server) |
| `/etc/ssh/sshd_config.d/*` | files | `sshd -t`, then reload |
| `/etc/sudoers.d/*` | files | mode 0440, checked with `visudo -c` before it's installed |
| `/etc/cron.d/*` | files | none (cron reads it); names can't contain dots |
| `/etc/sysctl.d/*` | files | `sysctl --system` |
| `/etc/systemd/{journald,timesyncd,resolved}.conf.d/*` | files | restart that daemon |
| `/etc/systemd/system/<unit>.d/*` | files | `daemon-reload`, restart the unit |
| `/etc/systemd/system/<unit>` | files | `daemon-reload`; with `[Install]`: enable, start and restart on change |
| `/etc/containers/systemd/<name>.{container,volume,network,…}` | files | installs Podman, `daemon-reload`; a `.container` with `[Install]` is started and restarted on change |
| `/etc/fail2ban/**` | files | check, then restart fail2ban |
| `/usr/local/{bin,sbin}/*` | files | mode 0755 |

A unit that's started also restarts when a file it reads through
`EnvironmentFile=` changes, if Trellis manages that file.

Not allowed: `/proc`, `/sys`, `/dev`, `/run`, `/tmp`, `/boot`, `/srv/www`
(deploys own it), and Trellis's own state. Paths Trellis already manages
are rejected with a pointer to their drop-in directory or `eject`.

## Commands

```yaml
defaults:            # or per environment
  commands:
    - run: fc-cache -f
      on: [/usr/local/share/fonts/]
    - name: tailscale-up
      run: tailscale up --auth-key="$TS_AUTHKEY"
      env: { TS_AUTHKEY: secret:tailscale_authkey }
```

- `run` is `/bin/sh -c`, as root, after everything else.
- It runs once, then again when `run` or `env` changes, or when any file under
  an `on:` path changes. `on:` paths must be files this manifest manages
  (yours or Trellis's), so the inputs are on the server before the command runs.
- `env` values may be `secret:<path>`; they're redacted in plans and history.
- A failure is reported and retried on the next apply. Removing a command
  stops tracking it; nothing is undone.
- `name` defaults to the first words of `run` (`fc-cache-f`).

Prefer a file whenever there is one: a command can't be planned or pruned.

## Recipes

### Scheduled job

```
server/files/etc/cron.d/example-reindex
```
```
0 3 * * * web cd /srv/www/example.com/current && wp search-index rebuild --quiet
```

For journal logging or randomized start times, use a systemd `.service`
without `[Install]` plus a `.timer` with `[Install]` under
`server/files/etc/systemd/system/`.

### Background worker

```ini
# server/files/etc/systemd/system/example-actions.service
[Service]
User=web
WorkingDirectory=/srv/www/example.com/current
ExecStart=/usr/local/bin/wp action-scheduler run --batches=100
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

No `trellis.yml` needed: `[Install]` makes it enabled and started. A worker
keeps the release it started with until it exits, so let it exit
regularly (here: after 100 batches; `Restart=always` brings it back on the
new release).

### Kernel and service limits

```
server/files/etc/sysctl.d/90-tuning.conf                           → sysctl --system
server/files/etc/systemd/system/php8.4-fpm.service.d/limits.conf   → daemon-reload, restart PHP-FPM
```

### Basic auth on staging only

```
server/env/staging/files/etc/nginx/includes.d/example.com/auth.conf
server/env/staging/files/etc/nginx/htpasswd/example.com.tmpl
```
```nginx
auth_basic "Staging";
auth_basic_user_file /etc/nginx/htpasswd/example.com;
```
```
{{ secret "basic_auth/user" }}:{{ secret "basic_auth/hash" }}
```

The htpasswd file is sensitive (0640, group `www-data`); nginx is checked and
reloaded.

### Third-party apt repository (Tailscale)

```
server/files/etc/apt/keyrings/tailscale.gpg          (committed: no network fetch)
server/files/etc/apt/sources.list.d/tailscale.sources
```
```yaml
packages: [tailscale]
commands:
  - name: tailscale-up
    run: tailscale up --auth-key="$TS_AUTHKEY" --ssh=false
    env: { TS_AUTHKEY: secret:tailscale_authkey }
```

The repository lands before packages install; the package starts its own
service.

### Containerized service (Podman Quadlet)

```ini
# server/files/etc/containers/systemd/meilisearch.container
[Container]
Image=docker.io/getmeili/meilisearch:v1.11@sha256:…
PublishPort=127.0.0.1:7700:7700
Volume=meili-data:/meili_data
EnvironmentFile=/etc/trellis/meilisearch.env

[Service]
MemoryMax=512M
Restart=always

[Install]
WantedBy=multi-user.target
```
```
# server/files/etc/trellis/meilisearch.env.tmpl
MEILI_MASTER_KEY={{ secret "meili_master_key" }}
```

Podman is installed, the unit is started, and it restarts when either file
changes. Pin the image by digest.

### Fonts

```
server/files/usr/local/share/fonts/brand/*.ttf
```
```yaml
commands:
  - run: fc-cache -f
    on: [/usr/local/share/fonts/]
```

### PHP extension

Prefer a package: `packages: [php8.4-excimer]`. Only without one:
`php8.4-dev` plus a `pecl install` command plus an ini file under
`/etc/php/8.4/mods-available/`. That's the escape hatch: never undone or
upgraded automatically.

## Not supported, by design

- Editing part of a file (`lineinfile`): use a `.d` directory, eject, or leave
  the file unmanaged.
- Ordering between custom steps, conditionals and loops.
- Undoing commands.
- Operations as desired state: backups, database imports, rolling restarts.
  Those are CLI commands.
- Anything off the server: DNS, CDNs, cloud resources.
- Users other than `admin` and `web`.
