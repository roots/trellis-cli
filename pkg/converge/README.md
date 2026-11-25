# converge: Ansible-free provisioning (experimental)

`trellis converge` provisions servers from a single `trellis.yml` without
Ansible. It keeps Trellis's opinions (the outcome) and replaces how they are
applied.

```
trellis.yml + trellis.lock + secrets/<env>.yml + server/
        │  compile (local, pure, offline)
        ▼
per-node manifest ── every file's bytes, package, unit, DB, cert, notification
        │  ssh (one multiplexed connection)
        ▼
trellis-agent on the node: observe → plan → (approve) → apply → record generation
```

## Two binaries

`trellis` (the CLI) does everything that needs the project: config, secrets
and compiling. `trellis-agent` (`cmd/trellis-agent`) runs on servers and
links only `agent`, `engine`, `host`, `manifest` and `state`, which a test
enforces. It is ~6.5MB (~2.7MB gzipped) and stdlib-only.

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
| `remote`    | SSH transport, agent upload, multi-node orchestration (`remote.Exec` injected)            | – |
| `project`   | Finds/loads a project and assembles a compile input                                       | – |

The CLI commands in `cmd/converge.go` are a thin layer over these packages.

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
- **engine**: `hosttest.Fake` simulates files, dpkg/apt, systemctl, MariaDB,
  gpg and certbot in memory. Tests run the full loop and assert:
  convergence (a second plan is empty), constant-cost no-op runs,
  drift repair, rollback when `nginx -t` fails, pre-write validation (`visudo`),
  pruning, recovery from failed runs, download verification.
- **remote**: a fake `ssh` dispatches to the real agent against fake hosts, so
  the client, protocol, agent upload and multi-node ordering are tested end to end.

## Resource kinds

`package`, `file` (whole files only; optional pre-write `validate`, `absent`),
`download` (sha256 or GPG fingerprint verified), `service`, `database`,
`database_user`, `cert` (self-signed in Go; Let's Encrypt via certbot webroot),
`exec` (escape hatch, re-runs only when its inputs hash changes).

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

## Not done yet

- `trellis converge lock` resolves GitHub keys only; package versions, image
  digests and tool checksums still need resolvers (compile warns when unpinned).
- nginx and MariaDB apt signing keys aren't fingerprint-pinned yet (Sury is).
- Deploys, the development VM integration (`--host` works with any VM), and
  importing existing `group_vars`.
- h5bp nginx includes are only partially ported.
