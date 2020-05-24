package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/mitchellh/cli"
)

const HookbookScript = `
# Hookbook (https://github.com/Shopify/hookbook)
#
# Copyright 2019 Shopify Inc.
#
# Permission is hereby granted, free of charge, to any person obtaining a copy of
# this software and associated documentation files (the "Software"), to deal in
# the Software without restriction, including without limitation the rights to
# use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
# the Software, and to permit persons to whom the Software is furnished to do so,
# subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
# FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
# COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
# IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
# CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

__hookbook_shell="$(\ps -p $$ | \awk 'NR > 1 { sub(/^-/, "", $4); print $4 }')"
__hookbook_shellname="$(basename "${__hookbook_shell}")"

__hookbook_array_contains() {
  [[ "$#" -lt 2 ]] && \return 1
  \local seeking="$1"; \shift
  \local check="$1"; \shift
  [[ "${seeking}" == "${check}" ]] && \return 0
  __hookbook_array_contains "${seeking}" "$@"
}

__hookbook_call_each() {
  [[ "$#" -lt 2 ]] && \return
  \local hookname="$1"; \shift
  \local fn="$1"; \shift
  "${fn}" "${hookname}"
  __hookbook_call_each "${hookname}" "$@"
}

[[ "${__hookbook_shellname}" == "zsh" ]] && {
  hookbook_add_hook() {
    \local fn="$1"

    \eval "
      __hookbook_${fn}_preexec() { ${fn} preexec }
      __hookbook_${fn}_precmd()  { ${fn} precmd }
    "

    __hookbook_array_contains "__hookbook_${fn}_preexec" "${preexec_functions[@]}" \
      || preexec_functions+=("__hookbook_${fn}_preexec")

    __hookbook_array_contains "__hookbook_${fn}_precmd" "${precmd_functions[@]}" \
      || precmd_functions+=("__hookbook_${fn}_precmd")
  }
}

[[ "${__hookbook_shellname}" == "bash" ]] && {
  declare -p __hookbook_functions >/dev/null 2>&1 || {
    __hookbook_functions=()
  }

  [[ "$(uname -s)" == "Darwin" ]] && {
    __dev_null_major="$(stat -f "%Hr" "/dev/null")"
    __stat_stderr='stat -f "%Hr" /dev/fd/2'
  } || {
    __dev_null_major="$(stat -c "%t" /dev/null)"
    __stat_stderr='stat -c "%t" "$(readlink -f "/dev/fd/2")"'
  }
  \eval "__hookbook_debug_handler() {
    [[ \"\${BASH_COMMAND}\" == \"\${PROMPT_COMMAND}\" ]] && \\return
    [[ \"\$(${__stat_stderr})\" == \"${__dev_null_major}\" ]] && \\return
    __hookbook_call_each preexec \"\${__hookbook_functions[@]}\"
  }"
  \unset __stat_stderr __dev_null_major

  __hookbook_debug_trap() {
    {
      [[ $- =~ x ]] && {
        \set +x
        __hookbook_debug_handler 2>&3
        \set -x
      } || {
        __hookbook_debug_handler 2>&3
      }
    } 4>&2 2>/dev/null 3>&4
  }

  \trap '__hookbook_debug_trap "$_"' DEBUG

  hookbook_add_hook() {
    \local fn="$1"

    [[ ! "${PROMPT_COMMAND}" == *" $fn "* ]] && {
      PROMPT_COMMAND="{
        [[ \$- =~ x ]] && {
          \set +x; ${fn} precmd 2>&3; \set -x;
        } || {
          ${fn} precmd 2>&3;
        }
      } 4>&2 2>/dev/null 3>&4;
      ${PROMPT_COMMAND}"
    }

    __hookbook_array_contains "${fn}" "${__hookbook_functions[@]}" \
      || __hookbook_functions+=("${fn}")
  }
}

[[ "${__hookbook_shellname}" != "zsh" ]] && [[ "${__hookbook_shellname}" != "bash" ]] && {
  >&2 \echo "hookbook is not compatible with your shell (${__hookbook_shell})"
  \unset __hookbook_shell __hookbook_shellname
  \return 1
}

\unset __hookbook_shell __hookbook_shellname
`

const ZshScript = `
__trellis_cli_hook() {
  local flags; flags=()
  if [[ "$1" == "zsh-preexec" ]]; then
    flags=(--silent)
  fi
  "@SELF@" venv hook "${flags[@]}" | source /dev/stdin
}

@HOOKBOOK@
hookbook_add_hook __trellis_cli_hook
`

const BashScript = `
__trellis_cli_hook() {
  local flags; flags=(--shellpid "$$")
  if [[ "$1" == "preexec" ]]; then
    flags+=(--silent)
  fi
  eval "$("@SELF@" venv hook "${flags[@]}")"
}

@HOOKBOOK@
hookbook_add_hook __trellis_cli_hook
`

type ShellInitCommand struct {
	UI cli.Ui
}

func (c *ShellInitCommand) Run(args []string) int {
	commandArgumentValidator := &CommandArgumentValidator{required: 1, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	var script string

	switch shell := args[0]; shell {
	case "zsh":
		script = ZshScript
	case "bash":
		script = BashScript
	default:
		c.UI.Error(fmt.Sprintf("Error: invalid shell name '%s'. Supported shells: bash, zsh", shell))
		c.UI.Output(c.Help())
		return 1
	}

	executable, _ := os.Executable()
	script = strings.Replace(script, "@SELF@", executable, -1)
	script = strings.Replace(script, "@HOOKBOOK@", HookbookScript, -1)
	c.UI.Output(script)

	return 0
}

func (c *ShellInitCommand) Synopsis() string {
	return "Prints a script which can be eval'd to set up Trellis' virtualenv integration in various shells."
}

func (c *ShellInitCommand) Help() string {
	helpText := `
Usage: trellis shell-init [options] SHELL

Prints a script which can be eval'd to set up Trellis' virtualenv integration in various shells.

To activate the integration, add one of the following lines to your shell profile (.zshrc, .bash_profile):

  eval "$(trellis shell-init bash)" # for bash
  eval "$(trellis shell-init zsh)"  # for zsh

Options:
  -h, --help  show this help
`
	return strings.TrimSpace(helpText)
}
