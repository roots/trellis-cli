__trellis_cli_hook() {
  local flags; flags=(--shellpid "$$")
  if [[ "$1" == "preexec" ]]; then
    flags+=(--silent)
  fi
  eval "$("@SELF@" venv hook "${flags[@]}")"
}

@HOOKBOOK@
hookbook_add_hook __trellis_cli_hook
