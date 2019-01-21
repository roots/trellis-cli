# trellis-cli

A command-line interface (CLI) for [Trellis](https://roots.io/trellis/) with autocompletion.

Manage your Trellis projects via the `trellis` command.

Warning: this project is very early in development so bug reports are welcome.

## macOS Quick Install (via Homebrew)

`brew install roots/tap/trellis-cli`

## Installation

1. Download the [latest release](https://github.com/roots/trellis-cli/releases/latest) for your OS
2. Unzip/untar
3. Put the unarchived binary in your `$PATH`
4. Run `trellis --autocomplete-install` to install shell autocompletions

Note: only macOS (darwin) builds are tested so far.

## Usage

Run `trellis` for the complete usage and help.

Support commands so far:

* `deploy` - Deploys a site to the specified environment.
* `galaxy` - Commands for Ansible Galaxy
* `info` - Displays information about this Trellis project
* `new` - Creates a new Trellis project
* `provision` - Provisions the specified environment
* `rollback` - Rollsback the last deploy of the site on the specified environment.

## Development

trellis-cli requires Go 1.11+ since it uses Go modules.

1. Make sure Go 1.11+ is installed (`brew install go` on OSX)
2. Clone the repo
3. Run `go build`

## Contributing

Contributions are welcome from everyone. We have [contributing guidelines](https://github.com/roots/guidelines/blob/master/CONTRIBUTING.md) to help you get started.

## Trellis sponsors

Help support our open-source development efforts by [becoming a patron](https://www.patreon.com/rootsdev).

<a href="https://kinsta.com/?kaid=OFDHAJIXUDIV"><img src="https://cdn.roots.io/app/uploads/kinsta.svg" alt="Kinsta" width="200" height="150"></a> <a href="https://www.harnessup.com/"><img src="https://cdn.roots.io/app/uploads/harness-software.svg" alt="Harness Software" width="200" height="150"></a> <a href="https://k-m.com/"><img src="https://cdn.roots.io/app/uploads/km-digital.svg" alt="KM Digital" width="200" height="150"></a> <a href="https://www.itineris.co.uk/"><img src="https://cdn.roots.io/app/uploads/itineris.svg" alt="itineris" width="200" height="150"></a> <a href="https://www.hebergeurweb.ca"><img src="https://cdn.roots.io/app/uploads/hebergeurweb.svg" alt="Hébergement Web Québec" width="200" height="150"></a>

## Community

Keep track of development and community news.

* Participate on the [Roots Discourse](https://discourse.roots.io/)
* Follow [@rootswp on Twitter](https://twitter.com/rootswp)
* Read and subscribe to the [Roots Blog](https://roots.io/blog/)
* Subscribe to the [Roots Newsletter](https://roots.io/subscribe/)
* Listen to the [Roots Radio podcast](https://roots.io/podcast/)
