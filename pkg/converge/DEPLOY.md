# Deploys

```sh
trellis converge deploy production example.com [--ref v1.2]
trellis converge rollback production example.com
```

Status: implemented with local builds and shell-script hooks (the two
decisions below). Tested end to end on a VM with Bedrock: about 7s per
deploy (36MB artifact). Package `deploy` builds; package `release` runs on
the server inside the agent.

Not done yet: uploading only what changed against the current release; a
`build` command for CI.

## Customizing

Per site, in `trellis.yml`:

```yaml
sites:
  example.com:
    writable: [web/app/uploads]   # shared between releases
    deploy:
      keep: 5                     # releases kept on servers
      update_db: true             # wp core update-db after the switch
      exclude: ["*.map", "web/app/themes/*/resources"]
```

A pattern without a slash matches any file or directory name; one with a
slash matches a path from the release root.

Hooks are shell scripts in `deploy-hooks/` (`sh -e`). Each gets
`TRELLIS_ENV`, `TRELLIS_SITE` and `TRELLIS_COMMIT`; branch on
`$TRELLIS_SITE` for per-site steps.

| Script | Runs |
|---|---|
| `build.sh` | locally in the build directory, after Composer (assets) |
| `release.sh` | on the server as `web` in the new release, before the switch; failing aborts |
| `after.sh` | on the server as `web` in `current`, after the switch |
| `done.sh` | locally after every server switched (notifications, CDN purges) |

## What Trellis does today

`deploy.yml` runs on the server, as `web`:

1. `git clone`/fetch the repo into `shared/source`. The server needs access
   to the repo: a deploy key or SSH agent forwarding.
2. `git archive` the branch (or `repo_subtree_path`) into `releases/<timestamp>`.
3. Render `.env` into the release, and copy `vendor/` from the current
   release to speed up composer.
4. `composer install` on the server. Composer credentials (`composer_authentications`
   in the vault) are written to the server.
5. Symlink shared paths (`uploads`) into the release.
6. Finalize: switch `current` to the release, keep 5 releases, then run
   `wp core update-db`, delete the theme-roots transient, flush rewrites,
   and reload PHP-FPM.
7. Hooks: Ansible task files at `{initialize,update,prepare,build,share,finalize}_{before,after}`.
   Typical use is building theme assets (`build-before`, often locally,
   then copying them up).

## History

Trellis builds on the server because it inherited Capistrano's model
(roots/trellis#95). Theme assets were the exception from the start: built
locally and copied up (#155, later the `build-before` hook). That hybrid
builds assets from the local working tree but code from the git ref, so
they can disagree. Building the whole release locally from one commit
removes that mismatch.

## Decision: where to build (local)

**A. Build locally, ship an artifact (chosen).** `trellis converge
deploy` builds the release on the machine running it:

- `git archive <ref>` from the repo (a clean commit, never the dirty
  working tree), then `composer install --no-dev -o` with the user's own
  Composer, credentials and cache. Build hooks (e.g. `npm run build`) run
  in the same directory.
- Composer runs with `--ignore-platform-reqs` (the laptop's PHP isn't
  the server's); the server runs Composer's `platform_check.php` with its
  own PHP before switching.
- The result is packed (`tar.gz`, without `.git` and `node_modules`), hashed, and uploaded once to a
  content-addressed path. The agent unpacks it into `releases/<id>`, links
  `shared/`, runs server hooks and switches `current`.

The case for it:

- No git credentials, deploy keys or Composer credentials on servers.
- No Composer or git needed on servers at all, and no builds competing
  with PHP for memory.
- The same artifact can be built in CI (`trellis converge build`) and
  deployed anywhere: deploys are reproducible and rollbacks are exact.
- Asset builds, which Trellis users already do locally, become ordinary
  build hooks instead of a copy step.

The costs:

- Needs PHP and Composer locally. Bedrock developers have both.
- Upload size: `vendor/` is typically 10–30MB compressed. Later this can
  upload only what changed against the current release.

**B. Build on the server (Trellis's model).** The agent fetches the repo and
runs Composer. It keeps parity, but carries the costs above: server
credentials, Composer on servers, and non-reproducible builds.

## The rest (common to A and B)

- **Releases:** `releases/<UTC timestamp>-<short sha>`, owned web:www-data.
  The `current` symlink switches atomically (temporary symlink, then
  rename). Keep 5. `trellis converge rollback <env> <site>` switches back.
- **Shared paths:** `.env` (already provisioned in `shared/`), `uploads`,
  and `writable` paths from `trellis.yml`.
- **Built-in steps after the switch:** `wp core update-db` (the network
  version for multisite, skipped if WordPress isn't installed) and deleting
  the theme-roots transient (warnings only), then a graceful PHP-FPM
  reload. Not yet: per-site switches for these, and `wp rewrite flush`.
- **Hooks:** shell scripts, no Ansible. Each runs from the release directory
  and gets `TRELLIS_ENV`, `TRELLIS_SITE`, `TRELLIS_RELEASE`, `TRELLIS_CURRENT`.
  - `deploy-hooks/build.sh` runs locally in the build directory, after
    Composer and before packing (assets).
  - `deploy-hooks/release.sh` runs on the server as `web`, after `shared/`
    is linked and before the switch (cache warmup, checks; failing aborts
    the deploy).
  - `deploy-hooks/after.sh` runs on the server as `web`, after the switch.
- **As web, without secrets:** deploys and rollbacks log in as `web` with an
  `access.deploy` key and run the agent as `web` (a Lima VM's user, or
  `--user`, reaches it through sudo). They read only `trellis.yml`, never the
  secrets, and need no sudo beyond web's one rule (reloading PHP-FPM). So CI
  needs exactly one secret, a deploy key. The agent is the one apply
  installed, or for a different trellis-cli version, web's own copy in
  `~/.local/lib/trellis`; artifacts upload to `~/.cache/trellis`. Both are
  in web's home, which the PHP sandbox hides.
- **Safety:** one deploy or provision at a time per server: deploys take
  `/var/lib/trellis/deploy.lock`, which apply also holds (PHP can't reach
  it). Any failure before the switch (unpacking, the PHP platform check,
  `release.sh`) removes the new release and leaves `current` untouched; an
  existing release directory is never reused or removed.
- **Agent protocol:** a `deploy` request referencing an uploaded artifact
  by hash. Progress streams like apply events.
- **Migration:** existing Trellis releases and `current` are compatible
  (same layout), so the first converge deploy just adds a release.
