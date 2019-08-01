# trellis-cli

[![CircleCI](https://circleci.com/gh/roots/trellis-cli.svg?style=svg)](https://circleci.com/gh/roots/trellis-cli)
![GitHub release](https://img.shields.io/github/release/roots/trellis-cli)

A command-line interface (CLI) for [Trellis](https://roots.io/trellis/) with autocompletion.

Manage your Trellis projects via the `trellis` command.

## Quick Install (macOS and Linux via Homebrew)

`brew install roots/tap/trellis-cli`

### Script

We also offer a quick script version:

`curl -sL https://roots.io/trellis/cli/get | bash`

## Installation

trellis-cli provides binary releases for a variety of OSes. These binary versions can be manually downloaded and installed.

1. Download the [latest release](https://github.com/roots/trellis-cli/releases/latest) or any [specific version](https://github.com/roots/trellis-cli/releases)
2. Unpack it (`tar -zxvf trellis_0.3.1_Linux_x86_64.tar.gz`)
3. Find the `trellis` binary in the unpacked directory, and move it to its desired destination (`mv trellis_0.3.1_Darwin_x86_64/trellis /usr/local/bin/trellis`)
4. Make sure the above path is in your `$PATH`
5. Run `trellis --autocomplete-install` to install shell autocompletions

## Usage

Run `trellis` for the complete usage and help.

Supported commands so far:

| Command | Description |
| --- | --- |
| `check` | Checks if Trellis requirements are met |
| `deploy` | Deploys a site to the specified environment|
| `droplet` | Commands for DigitalOcean Droplets |
| `galaxy` | Commands for Ansible Galaxy |
| `info` | Displays information about this Trellis project |
| `new` | Creates a new Trellis project |
| `provision` | Provisions the specified environment |
| `rollback` | Rollsback the last deploy of the site on the specified environment |
| `vault` | Commands for Ansible Vault |

## Development

trellis-cli requires Go 1.11+ since it uses Go modules.

1. Make sure Go 1.11+ is installed (`brew install go` on macOS)
2. Clone the repo
3. Run `go build`
4. To run tests: `go test -v ./...`

## Contributing

Contributions are welcome from everyone. We have [contributing guidelines](https://github.com/roots/guidelines/blob/master/CONTRIBUTING.md) to help you get started.

## Trellis sponsors

Help support our open-source development efforts by [becoming a patron](https://www.patreon.com/rootsdev).

<a href="https://kinsta.com/?kaid=OFDHAJIXUDIV"><img src="https://cdn.roots.io/app/uploads/kinsta.svg" alt="Kinsta" width="200" height="150"></a> <a href="https://k-m.com/"><img src="https://cdn.roots.io/app/uploads/km-digital.svg" alt="KM Digital" width="200" height="150"></a> <a href="https://www.hebergeurweb.ca"><img src="https://cdn.roots.io/app/uploads/hebergeurweb.svg" alt="Hébergement Web Québec" width="200" height="150"></a>

## Community

Keep track of development and community news.

* Participate on the [Roots Discourse](https://discourse.roots.io/)
* Follow [@rootswp on Twitter](https://twitter.com/rootswp)
* Read and subscribe to the [Roots Blog](https://roots.io/blog/)
* Subscribe to the [Roots Newsletter](https://roots.io/subscribe/)
* Listen to the [Roots Radio podcast](https://roots.io/podcast/)
