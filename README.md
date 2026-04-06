# trellis-cli (WSL2 Fork)

> **This is a fork of [roots/trellis-cli](https://github.com/roots/trellis-cli)** that adds native **WSL2 virtual machine support for Windows**. The upstream CLI supports Lima (macOS/Linux). This fork adds a `wsl` backend that manages WSL2 distros via `wsl.exe`, giving Windows developers a first-class Trellis development experience.

[![Upstream](https://img.shields.io/badge/upstream-roots%2Ftrellis--cli-blue?style=flat-square)](https://github.com/roots/trellis-cli)

---

## What's New: WSL2 VM Backend

### Overview

Windows developers can now run `trellis vm start` to get a fully provisioned Trellis development environment powered by WSL2. Each project gets its own isolated Ubuntu distro with nginx, PHP-FPM, MariaDB, and all Trellis services — no manual WSL setup required.

### Requirements

- **Windows 11** with WSL2 enabled
- **VS Code** with the [WSL extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-wsl) (required for editing project files)
- **trellis-cli** (this fork)

> **Important:** Project files live on WSL2's native ext4 filesystem for performance. You must use an editor that supports WSL remote development. VS Code with the WSL extension is the recommended (and automated) path. JetBrains IDEs also support WSL remoting but are not automated by `vm open`.

### Quick Start

```powershell
# Create a new Trellis project (from PowerShell)
trellis new mysite.com

# Start the VM (imports Ubuntu, bootstraps Ansible, provisions everything)
cd mysite.com
trellis vm start

# Open VS Code connected to the WSL distro
trellis vm open

# From the VS Code integrated terminal (inside WSL):
trellis provision development    # Re-provision
trellis db open --app=tableplus  # Open database in TablePlus
```

### Windows/WSL Development Workflow

> **Important for Windows developers:** The development workflow differs slightly from macOS/Lima. On macOS, project files live on a shared filesystem, so dependency installs (composer, yarn) and frontend build tools run on the host. On WSL2, project files live on the distro's native ext4 filesystem for performance. **All dependency installs and build commands should be run inside the WSL terminal** (via `trellis vm open` or `trellis vm shell`).

After `vm start` and `vm open`, run your project's setup steps from the VS Code integrated terminal:

```bash
# Example: typical Bedrock + Sage project
cd site && composer install
cd web/app/themes/my-theme && composer install && yarn install
yarn dev   # Frontend asset watcher
```

Node.js LTS and Corepack (yarn/pnpm) are pre-installed in every WSL distro. You do **not** need Node.js on Windows for Trellis development. If your project requires additional CLI tools, you can install them directly in your WSL distro via `trellis vm shell` or the VS Code terminal.

### Commands

**New commands** (WSL2 only):

| Command    | Run From | Description                                                   |
|------------|----------|---------------------------------------------------------------|
| `vm open`  | Windows  | Opens VS Code connected to the WSL distro at the project root |
| `vm sync`  | Windows  | Manually syncs project files from WSL back to Windows         |
| `vm trust` | Windows  | Re-imports self-signed SSL certs into the Windows trust store |

**Enhanced for WSL2** (existing commands with added Windows-specific behavior):

| Command           | What Changed                                                                      |
|-------------------|-----------------------------------------------------------------------------------|
| `vm start`        | WSL2 backend: imports Ubuntu distro, bootstraps to ext4, auto-stops other distros |
| `vm stop`         | Auto SyncBack (rsync ext4 → Windows) before terminating the distro                |
| `vm delete`       | Cleans up Windows hosts file entries and SSL certs                                |
| `vm shell`        | Routes to WSL distro; detects when run from wrong host                            |
| `db open`         | Works from both Windows and WSL; uses direct `mysql://` URI (no SSH tunnels)      |
| `provision`       | Detects Windows host and redirects to WSL terminal                                |
| `deploy`          | Detects Windows host and redirects to WSL terminal                                |
| `vault *`         | Detects Windows host and redirects to WSL terminal                                |
| `galaxy install`  | Detects Windows host and redirects to WSL terminal                                |
| `xdebug-tunnel *` | Detects Windows host and redirects to WSL terminal                                |

### How It Works

1. **`vm start`** imports an Ubuntu rootfs into a dedicated WSL2 distro (e.g., `trellis-mysite-com`), installs Python/Ansible, copies the project to ext4, runs `ansible-playbook dev.yml`, tunes opcache, trusts SSL certs, and updates the Windows hosts file.

2. **Project files live on ext4** at `/home/admin/<project>/` inside the distro. This gives native filesystem performance (~80ms page loads vs ~14 seconds with Windows filesystem mounts). The `site/` directory is bind-mounted to `/srv/www/<site>/current` as Trellis expects.

3. **`vm open`** launches VS Code with `--folder-uri vscode-remote://wsl+<distro>/home/admin/<project>`, connecting directly to the WSL distro. The developer sees the full project (trellis/ + site/ + .git/) and uses git normally from the VS Code terminal.

4. **`vm stop`** runs an incremental rsync from WSL ext4 back to the Windows filesystem before stopping the distro, keeping the Windows-side repo up to date for GitHub Desktop or other Windows git tools.

5. **Smart command routing** — Ansible-dependent commands (provision, deploy, vault, etc.) detect when you're on the Windows host and tell you to run them from the WSL terminal instead. VM management commands detect when you're inside WSL and redirect you to Windows.

### Features

- **Ext4-native performance** — ~80ms TTFB (vs ~14s with DrvFS/9p bind mounts)
- **Automatic hosts file management** — Adds/removes `*.test` entries in the Windows hosts file (UAC elevation, only when entries change)
- **SSL certificate trust** — Self-signed certs auto-imported into the Windows Trusted Root CA store (sites must have `ssl.enabled: true` in `wordpress_sites.yml`)
- **Bi-directional file sync** — Auto sync on stop; manual `vm sync`; config auto-sync on any Windows-side command
- **Database GUI support** — `db open --app=tableplus` works from both Windows and WSL terminals, using direct `mysql://` URIs (no SSH tunnels needed)
- **Cross-compiled Linux binary** — Automatically deployed into distros so `trellis` commands work from the WSL terminal
- **Distro isolation** — Each project gets its own WSL distro; multiple projects can run simultaneously
- **Resilient lifecycle** — Detects unprovisioned distros and auto-cleans; keepalive process prevents WSL idle shutdown

### Architecture

```
Windows Host                          WSL2 Distro (trellis-mysite-com)
─────────────                         ─────────────────────────────────
trellis vm start ──────────────────── wsl --import → bootstrap → provision
trellis vm open  ──────────────────── code --folder-uri vscode-remote://wsl+...
trellis vm stop  ── rsync ext4→Win ── wsl -t (terminate)
trellis vm trust ── certutil ───────── reads /etc/nginx/ssl/*.cert
trellis db open  ── rundll32 URI ───── ansible-playbook → JSON credentials

C:\Users\...\mysite.com\             /home/admin/mysite.com/
  trellis/  (config, read by Win)       trellis/  (config, used by Ansible)
  site/     (Windows backup)            site/     (ext4, served by nginx)
  .git/     (Windows backup)            .git/     (ext4, used by VS Code)
```

### Configuration

The WSL backend is auto-selected on Windows. You can explicitly set it in `trellis.cli.yml`:

```yaml
vm:
  manager: "wsl"    # "auto" also works (selects wsl on Windows, lima on macOS)
  ubuntu: "24.04"   # Ubuntu version for the rootfs (22.04 or 24.04)
```

### Differences from Lima (macOS)

| Aspect             | Lima (macOS)            | WSL2 (Windows)                |
|--------------------|-------------------------|-------------------------------|
| VM technology      | QEMU/Lima               | WSL2 (Hyper-V)                |
| Filesystem         | virtiofs (FUSE)         | ext4 native                   |
| Networking         | Lima port forwarding    | WSL2 NAT (automatic)          |
| Editor requirement | Any (shared filesystem) | VS Code + WSL extension       |
| SSH                | Lima manages SSH keys   | Not needed (local connection) |
| Ansible connection | `local`                 | `local`                       |

### Known Limitations

- **One project at a time** — All WSL2 distros share a single network stack ([by design](https://github.com/microsoft/WSL/issues/4304)), so services like MariaDB (3306), nginx (80/443), and openssh-server (22) conflict if multiple distros run simultaneously. `vm start` automatically stops other `trellis-*` distros before starting yours, with an optional SyncBack prompt so you can sync unsaved work back to Windows first. Your data is safe — stopped distros resume exactly where they left off.
- **VS Code is required** for editing project files (they live on WSL2 ext4, not the Windows filesystem)
- **Windows-side files are a backup** — the Windows copy is kept in sync by `vm stop` and `vm sync` but is not the source of truth during development
- **One UAC prompt** per `vm start` (for hosts file and SSL cert trust) — subsequent starts skip UAC if entries haven't changed

---

## Upstream README

*Everything below is from the original [roots/trellis-cli](https://github.com/roots/trellis-cli).*

---

# trellis-cli

[![Build status]( https://img.shields.io/github/actions/workflow/status/roots/trellis-cli/ci.yml?branch=master&style=flat-square)](https://github.com/roots/trellis-cli/actions)
![GitHub release](https://img.shields.io/github/release/roots/trellis-cli?style=flat-square)
[![Follow Roots](https://img.shields.io/badge/follow%20@rootswp-1da1f2?logo=twitter&logoColor=ffffff&message=&style=flat-square)](https://twitter.com/rootswp)
[![Sponsor Roots](https://img.shields.io/badge/sponsor%20roots-525ddc?logo=github&style=flat-square&logoColor=ffffff&message=)](https://github.com/sponsors/roots)

A command-line interface (CLI) to manage [Trellis](https://roots.io/trellis/) projects via the `trellis` command. It includes:
* Smart autocompletion (based on your defined environments and sites)
* Automatic Virtualenv integration for easier dependency management
* Easy [DigitalOcean](https://roots.io/r/digitalocean) droplet creation
* Better Ansible Vault support for encrypting files

## Support us

Roots is an independent open source org, supported only by developers like you. Your sponsorship funds [WP Packages](https://wp-packages.org/) and the entire Roots ecosystem, and keeps them independent. Support us by purchasing [Radicle](https://roots.io/radicle/) or [sponsoring us on GitHub](https://github.com/sponsors/roots) — sponsors get access to our private Discord.

### Sponsors

<a href="https://carrot.com/"><img src="https://cdn.roots.io/app/uploads/carrot.svg" alt="Carrot" width="120" height="90"></a> <a href="https://wordpress.com/"><img src="https://cdn.roots.io/app/uploads/wordpress.svg" alt="WordPress.com" width="120" height="90"></a> <a href="https://www.itineris.co.uk/"><img src="https://cdn.roots.io/app/uploads/itineris.svg" alt="Itineris" width="120" height="90"></a> <a href="https://kinsta.com/?kaid=OFDHAJIXUDIV"><img src="https://cdn.roots.io/app/uploads/kinsta.svg" alt="Kinsta" width="120" height="90"></a>

## Quick Install (macOS and Linux via Homebrew)

`brew install roots/tap/trellis-cli`

### Upgrading to latest stable install via Homebrew

```bash
brew update
brew upgrade trellis-cli
```

## Quick Install (Unstable - macOS and Linux via Homebrew)

```bash
# Cleanup previous versions (if installed)
brew uninstall roots/tap/trellis-cli

# Install
brew install --HEAD roots/tap/trellis-cli-dev

# Upgrade
brew upgrade --fetch-HEAD roots/tap/trellis-cli-dev
```

### Script

We also offer a quick script version:

```bash
# You might need sudo before bash
curl -sL https://roots.io/trellis/cli/get | bash

# Turns on debug logging
curl -sL https://roots.io/trellis/cli/get | bash -s -- -d

# Sets bindir or installation directory, Defaults to '/usr/local/bin'
curl -sL https://roots.io/trellis/cli/get | bash -s -- -b /path/to/my/bin
```

## Manual Install

trellis-cli provides binary releases for a variety of OSes. These binary versions can be manually downloaded and installed.

1. Download the [latest release](https://github.com/roots/trellis-cli/releases/latest) or any [specific version](https://github.com/roots/trellis-cli/releases)
2. Unpack it (`tar -zxvf trellis_1.0.0_Linux_x86_64.tar.gz`)
3. Find the `trellis` binary in the unpacked directory, and move it to its desired destination (`mv trellis_1.0.0_Darwin_x86_64/trellis /usr/local/bin/trellis`)
4. Make sure the above path is in your `$PATH`

## Windows Install
trellis-cli does offer a native Windows exe but we [recommend you use
WSL](https://roots.io/trellis/docs/installation/#local-development-requirements) for Trellis. The above install methods will work for WSL as well.

If you do want to use the native Windows exe, you'll need to do the following
setup after downloading the Windows build:

1. Open system properties
2. Open environment variables
3. Under system variables add new variable, `TRELLIS`, pointing to the location of the `trellis.exe` file, like `C:\trellis_1.0.0`
4. Edit path from system variables and add new named `%TRELLIS%`
5. Save the changes

## Verify Attestation
trellis-cli artifacts can be [cryptographically verified via GitHub CLI](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/using-artifact-attestations-to-establish-provenance-for-builds#verifying-artifact-attestations-with-the-github-cli).

```console
# The archive with both predicates
$ gh attestation verify --repo roots/trellis-cli /path/to/trellis_Darwin_arm64.tar.gz
## ...snipped...
✓ Verification succeeded!

sha256:xxx was attested by:
REPO                   PREDICATE_TYPE                  WORKFLOW
roots/trellis-cli  https://slsa.dev/provenance/v1  .github/workflows/release.yml@refs/tags/v9.8.7
roots/trellis-cli  https://spdx.dev/Document/v2.3  .github/workflows/release.yml@refs/tags/v9.8.7

# The binary
$ gh attestation verify --repo roots/trellis-cli /path/to/trellis
## ...snipped...
✓ Verification succeeded!

sha256:xxx was attested by:
REPO                   PREDICATE_TYPE                  WORKFLOW
roots/trellis-cli  https://slsa.dev/provenance/v1  .github/workflows/release.yml@refs/tags/v9.8.7

# The SBOM
$ gh attestation verify --repo roots/trellis-cli /path/to/trellis_Darwin_arm64.tar.gz.sbom.json
## ...snipped...
✓ Verification succeeded!

sha256:xxx was attested by:
REPO                   PREDICATE_TYPE                  WORKFLOW
roots/trellis-cli  https://slsa.dev/provenance/v1  .github/workflows/release.yml@refs/tags/v9.8.7
```

## Shell Integration

### Autocompletes

Homebrew installs trellis-cli's shell completion automatically by default. If shell completions aren't working, or you installed manually not using Homebrew, you'll need to install the completions manually.

To use the trellis-cli's autocomplete via Homebrew's shell completion:

1. Follow Homebrew's install instructions https://docs.brew.sh/Shell-Completion

    Note: For zsh, as the instructions mention, be sure compinit is autoloaded and called, either explicitly or via a framework like oh-my-zsh.

2. Then run:

    ```bash
    brew reinstall trellis-cli
    ```

To install shell completions manually, run the following:

```bash
trellis --autocomplete-install
```

It should modify your `.bash_profile`, `.zshrc` or similar.

### Virtualenv

trellis-cli uses [Virtualenv](https://virtualenv.pypa.io) to manage dependencies such as Ansible which it automatically activates and uses when running any `trellis` command.
But there's still a lot of times you may want to run `ansible-playbook` or `pip` manually in your shell. To make this experience seamless, trellis-cli
offers shell integration which automatically activates the Virtualenv when you enter a Trellis project, and deactivates when you leave it.

![venv integration](https://user-images.githubusercontent.com/295605/84097210-d8df6700-a9d1-11ea-9eaf-fbdbd6632d34.gif)

To enable this integration, add the following to your shell profile:

Bash (`~/.bash_profile`):
```bash
eval "$(trellis shell-init bash)"
```

Zsh (`~/.zshrc`):
```bash
eval "$(trellis shell-init zsh)"
```

## Usage

Run `trellis` for the complete usage and help.

Supported commands so far:

| Command         | Description                                                                                               |
|-----------------|-----------------------------------------------------------------------------------------------------------|
| `alias`         | Generate WP CLI aliases for remote environments                                                           |
| `check`         | Checks if Trellis requirements are met                                                                    |
| `db`            | Commands for database management                                                                          |
| `deploy`        | Deploys a site to the specified environment                                                               |
| `dotenv`        | Template .env files to local system                                                                       |
| `droplet`       | Commands for DigitalOcean Droplets                                                                        |
| `exec`          | Exec runs a command in the Trellis virtualenv                                                             |
| `galaxy`        | Commands for Ansible Galaxy                                                                               |
| `info`          | Displays information about this Trellis project                                                           |
| `init`          | Initializes an existing Trellis project                                                                   |
| `key`           | Commands for managing SSH keys                                                                            |
| `logs`          | Tails the Nginx log files                                                                                 |
| `new`           | Creates a new Trellis project                                                                             |
| `open`          | Opens user-defined URLs (and more) which can act as shortcuts/bookmarks specific to your Trellis projects |
| `provision`     | Provisions the specified environment                                                                      |
| `rollback`      | Rollsback the last deploy of the site on the specified environment                                        |
| `ssh`           | Connects to host via SSH                                                                                  |
| `valet`         | Commands for Laravel Valet                                                                                |
| `vault`         | Commands for Ansible Vault                                                                                |
| `vm`            | Commands for local development virtual machines                                                           |
| `xdebug-tunnel` | Commands for managing Xdebug tunnels                                                                      |

## Configuration
There are three ways to set configuration settings for trellis-cli and they are
loaded in this order of precedence:

1. global config (`$HOME/.config/trellis/cli.yml`)
2. project config (`trellis.cli.yml`)
3. project config local override (`trellis.cli.local.yml`)
4. env variables

The global CLI config (defaults to `$HOME/.config/trellis/cli.yml`)
and will be loaded first (if it exists).

Next, if a project is detected, the project CLI config will be loaded if it
exists at `trellis.cli.yml`. A Git ignored local override config is also
supported at `trellis.cli.local.yml`.

Finally, env variables prefixed with `TRELLIS_` will be used as
overrides if they match a supported configuration setting. The prefix will be
stripped and the rest is lowercased to determine the setting key.

Note: only string, numeric, and boolean values are supported when using environment
variables.

Current supported settings:

| Setting                     | Description                                                           | Type              | Default   |
|-----------------------------|-----------------------------------------------------------------------|-------------------|-----------|
| `allow_development_deploys` | Whether to allows deploy to the `development` env                     | boolean           | false     |
| `ask_vault_pass`            | Set Ansible to always ask for the vault pass                          | boolean           | false     |
| `check_for_updates`         | Whether to check for new versions of trellis-cli                      | boolean           | true      |
| `database_app`              | Database app to use in `db open` (Options: `tableplus`, `sequel-ace`) | string            | none      |
| `load_plugins`              | Load external CLI plugins                                             | boolean           | true      |
| `open`                      | List of name -> URL shortcuts                                         | map[string]string | none      |
| `virtualenv_integration`    | Enable automated virtualenv integration                               | boolean           | true      |
| `vm`                        | Options for dev virtual machines                                      | Object            | see below |

### `vm`
| Setting   | Description                                                 | Type   | Default |
|-----------|-------------------------------------------------------------|--------|---------|
| `manager` | VM manager (Options: `auto` (depends on OS), `lima`, `wsl`) | string | "auto"  |
| `ubuntu` | Ubuntu OS version (Options: `22.04`, `24.04`)| string |
| `hosts_resolver` | VM hosts resolver (Options: `hosts_file`)| string |
| `instance_name` | Custom name for the VM instance | string | First site name alphabetically |
| `images` | Custom OS image | object | Set based on `ubuntu` version |

#### `images`
| Setting    | Description                                     | Type   | Default |
|------------|-------------------------------------------------|--------|---------|
| `location` | URL of Ubuntu image                             | string | none    |
| `arch`     | Architecture of image (eg: `x86_64`, `aarch64`) | string | none    |

Example config:

```yaml
ask_vault_pass: false
check_for_updates: true
load_plugins: true
open:
  site: "https://mysite.com"
  admin: "https://mysite.com/wp/wp-admin"
virtualenv_integration: true
vm:
  manager: "lima"
  instance_name: "custom-instance-name"  # Optional: Set a specific VM instance name
```

Example env var usage:
```bash
TRELLIS_ASK_VAULT_PASS=true trellis provision production
```

## Development

trellis-cli requires Go >= 1.18 (`brew install go` on macOS)

```bash
# Clone the repo
git clone https://github.com/roots/trellis-cli
cd trellis-cli

# Build the binary for your machine
go build

# Run tests (without integration tests)
go test -v -short ./...

# (Optional) Build the docker image for testing (requires `docker`)
make docker
# Alternatively, do not use cache when building the doccker image (requires `docker`)
make docker-no-cache

# Run all tests (requires `docker`)
make test
```

## Releasing Docker Images

*This section only intended for the maintainers*

```bash
make docker-no-cache

# docker tag rootsdev/trellis-cli-dev:latest rootsdev/trellis-cli-dev:YYYY.MM.DD.N
# where N is a sequential integer, starting from 1.
docker tag rootsdev/trellis-cli-dev:latest rootsdev/trellis-cli-dev:2019.08.12.1

# docker push rootsdev/trellis-cli-dev:YYYY.MM.DD.N
docker push rootsdev/trellis-cli-dev:2019.08.12.1
docker push rootsdev/trellis-cli-dev:latest
```

## Community

Keep track of development and community news.

- Join us on Discord by [sponsoring us on GitHub](https://github.com/sponsors/roots)
- Join us on [Roots Discourse](https://discourse.roots.io/)
- Follow [@rootswp on Twitter](https://twitter.com/rootswp)
- Follow the [Roots Blog](https://roots.io/blog/)
- Subscribe to the [Roots Newsletter](https://roots.io/subscribe/)

