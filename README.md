# trellis-cli

[![Build status]( https://img.shields.io/github/actions/workflow/status/roots/trellis-cli/ci.yml?branch=master&style=flat-square)](https://github.com/roots/trellis-cli/actions)
![GitHub release](https://img.shields.io/github/release/roots/trellis-cli?style=flat-square)
[![Sponsor Roots](https://img.shields.io/badge/sponsor%20roots-525ddc?logo=github&style=flat-square&logoColor=ffffff&message=)](https://github.com/sponsors/roots)

A command-line interface (CLI) to manage [Trellis](https://roots.io/trellis/) projects via the `trellis` command. It includes:
* Smart autocompletion (based on your defined environments and sites)
* Automatic Virtualenv integration for easier dependency management
* Easy [DigitalOcean](https://roots.io/r/digitalocean) droplet creation
* Better Ansible Vault support for encrypting files

## Support us

We're dedicated to pushing modern WordPress development forward through our open source projects, and we need your support to keep building. You can support our work by purchasing [Radicle](https://roots.io/radicle/), our recommended WordPress stack, or by [sponsoring us on GitHub](https://github.com/sponsors/roots). Every contribution directly helps us create better tools for the WordPress ecosystem.

### Sponsors

<a href="https://carrot.com/"><img src="https://cdn.roots.io/app/uploads/carrot.svg" alt="Carrot" width="120" height="90"></a> <a href="https://wordpress.com/"><img src="https://cdn.roots.io/app/uploads/wordpress.svg" alt="WordPress.com" width="120" height="90"></a> <a href="https://www.itineris.co.uk/"><img src="https://cdn.roots.io/app/uploads/itineris.svg" alt="Itineris" width="120" height="90"></a> <a href="https://bonsai.so/"><img src="https://cdn.roots.io/app/uploads/bonsai.svg" alt="Bonsai" width="120" height="90"></a>

## Quick Install (macOS and Linux via Homebrew)

`brew install roots/tap/trellis-cli`

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

| Command | Description |
| --- | --- |
| `alias` | Generate WP CLI aliases for remote environments |
| `check` | Checks if Trellis requirements are met |
| `db` | Commands for database management |
| `deploy` | Deploys a site to the specified environment |
| `dotenv` | Template .env files to local system |
| `down` | Stops the Vagrant machine by running `vagrant halt`|
| `droplet` | Commands for DigitalOcean Droplets |
| `exec` | Exec runs a command in the Trellis virtualenv |
| `galaxy` | Commands for Ansible Galaxy |
| `info` | Displays information about this Trellis project |
| `init` | Initializes an existing Trellis project |
| `key` | Commands for managing SSH keys |
| `logs` | Tails the Nginx log files |
| `new` | Creates a new Trellis project |
| `open` | Opens user-defined URLs (and more) which can act as shortcuts/bookmarks specific to your Trellis projects |
| `provision` | Provisions the specified environment |
| `rollback` | Rollsback the last deploy of the site on the specified environment |
| `ssh` | Connects to host via SSH |
| `up` | Starts and provisions the Vagrant environment by running `vagrant up` |
| `valet` | Commands for Laravel Valet |
| `vault` | Commands for Ansible Vault |
| `xdebug-tunnel` | Commands for managing Xdebug tunnels |

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

| Setting | Description | Type | Default |
| --- | --- | -- | -- |
| `allow_development_deploys` | Whether to allows deploy to the `development` env | boolean | false |
| `ask_vault_pass` | Set Ansible to always ask for the vault pass | boolean | false |
| `check_for_updates` | Whether to check for new versions of trellis-cli | boolean | true |
| `database_app` | Database app to use in `db open` (Options: `tableplus`, `sequel-ace`)| string | none |
| `load_plugins` | Load external CLI plugins | boolean | true |
| `open` | List of name -> URL shortcuts | map[string]string | none |
| `virtualenv_integration` | Enable automated virtualenv integration | boolean | true |
| `vm` | Options for dev virtual machines | Object | see below |

### `vm`
| Setting | Description | Type | Default |
| --- | --- | -- | -- |
| `manager` | VM manager (Options: `auto` (depends on OS), `lima`)| string | "auto" |
| `ubuntu` | Ubuntu OS version (Options: `18.04`, `20.04`, `22.04`, `24.04`)| string |
| `hosts_resolver` | VM hosts resolver (Options: `hosts_file`)| string |
| `instance_name` | Custom name for the VM instance | string | First site name alphabetically |
| `images` | Custom OS image | object | Set based on `ubuntu` version |

#### `images`
| Setting | Description | Type | Default |
| --- | --- | -- | -- |
| `location` | URL of Ubuntu image | string | none |
| `arch` | Architecture of image (eg: `x86_64`, `aarch64`) | string | none |

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

## Contributing

Contributions are welcome from everyone. We have [contributing guidelines](https://github.com/roots/guidelines/blob/master/CONTRIBUTING.md) to help you get started.

## Community

Keep track of development and community news.

- Join us on Discord by [sponsoring us on GitHub](https://github.com/sponsors/roots)
- Participate on the [Roots Discourse](https://discourse.roots.io/)
- Follow [@rootswp on Twitter](https://twitter.com/rootswp)
- Read and subscribe to the [Roots Blog](https://roots.io/blog/)
- Subscribe to the [Roots Newsletter](https://roots.io/subscribe/)
