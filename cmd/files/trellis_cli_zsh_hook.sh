__trellis_cli_hook() {
  local flags; flags=()
  if [[ "$1" == "zsh-preexec" ]]; then
    flags=(--silent)
  fi
  "@SELF@" venv hook "${flags[@]}" | source /dev/stdin
}

@HOOKBOOK@
hookbook_add_hook __trellis_cli_hook
