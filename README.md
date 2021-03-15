# trellis-cli

[![CircleCI](https://circleci.com/gh/roots/trellis-cli.svg?style=svg)](https://circleci.com/gh/roots/trellis-cli)
![GitHub release](https://img.shields.io/github/release/roots/trellis-cli)

A command-line interface (CLI) to manage [Trellis](https://roots.io/trellis/) projects via the `trellis` command. It includes:
* Smart autocompletion (based on your defined environments and sites)
* Automatic Virtualenv integration for easier dependency management
* Easy [DigitalOcean](https://roots.io/r/digitalocean) droplet creation
* Better Ansible Vault support for encrypting files

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
2. Unpack it (`tar -zxvf trellis_0.9.0_Linux_x86_64.tar.gz`)
3. Find the `trellis` binary in the unpacked directory, and move it to its desired destination (`mv trellis_0.9.0_Darwin_x86_64/trellis /usr/local/bin/trellis`)
4. Make sure the above path is in your `$PATH`

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
| `new` | Creates a new Trellis project |
| `provision` | Provisions the specified environment |
| `rollback` | Rollsback the last deploy of the site on the specified environment |
| `ssh` | Connects to host via SSH |
| `up` | Starts and provisions the Vagrant environment by running `vagrant up` |
| `valet` | Commands for Laravel Valet |
| `vault` | Commands for Ansible Vault |

## Development

trellis-cli requires Go >= 1.13 (`brew install go` on macOS)

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

## Trellis sponsors

Help support our open-source development efforts by [becoming a patron](https://www.patreon.com/rootsdev).

<a href="https://kinsta.com/?kaid=OFDHAJIXUDIV"><img src="https://cdn.roots.io/app/uploads/kinsta.svg" alt="Kinsta" width="200" height="150"></a> <a href="https://k-m.com/"><img src="https://cdn.roots.io/app/uploads/km-digital.svg" alt="KM Digital" width="200" height="150"></a> <a href="https://carrot.com/"><img src="https://cdn.roots.io/app/uploads/carrot.svg" alt="Carrot" width="200" height="150"></a> <a href="https://www.c21redwood.com/"><img src="https://cdn.roots.io/app/uploads/c21redwood.svg" alt="C21 Redwood Realty" width="200" height="150"></a> <a href="https://wordpress.com/"><img src="https://cdn.roots.io/app/uploads/wordpress.svg" alt="WordPress.com" width="200" height="150"></a>

## Community

Keep track of development and community news.

* Participate on the [Roots Discourse](https://discourse.roots.io/)
* Follow [@rootswp on Twitter](https://twitter.com/rootswp)
* Read and subscribe to the [Roots Blog](https://roots.io/blog/)
* Subscribe to the [Roots Newsletter](https://roots.io/subscribe/)
* Listen to the [Roots Radio podcast](https://roots.io/podcast/)
