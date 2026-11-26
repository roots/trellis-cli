# Deploys: design (draft)

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

## Decision: where to build

**A. Build locally, ship an artifact (recommended).** `trellis converge
deploy` builds the release on the machine running it:

- `git archive <ref>` from the repo (a clean commit, never the dirty
  working tree), then `composer install --no-dev -o` with the user's own
  Composer, credentials and cache. Build hooks (e.g. `npm run build`) run
  in the same directory.
- Composer's `config.platform` is set to the server's PHP version, so the
  lock resolves for the server, not the laptop.
- The result is packed (`tar.zst`), hashed, and uploaded once to a
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
- **Built-in steps after the switch** (each can be turned off per site):
  `wp core update-db` (or the network version for multisite), delete the
  theme-roots transient, `wp rewrite flush`, and a graceful PHP-FPM reload.
- **Hooks:** shell scripts, no Ansible. Each runs from the release directory
  and gets `TRELLIS_ENV`, `TRELLIS_SITE`, `TRELLIS_RELEASE`, `TRELLIS_CURRENT`.
  - `deploy/build.sh` runs locally, after Composer and before packing
    (assets).
  - `deploy/release.sh` runs on the server as `web`, after `shared/` is
    linked and before the switch (cache warmup, checks; failing aborts
    the deploy).
  - `deploy/after.sh` runs on the server as `web`, after the switch.
- **Safety:** one deploy per site at a time (lock). A failed hook or
  `update-db` before the switch leaves `current` untouched.
- **Agent protocol:** a `deploy` request referencing an uploaded artifact
  by hash. Progress streams like apply events.
- **Migration:** existing Trellis releases and `current` are compatible
  (same layout), so the first converge deploy just adds a release.
