#compdef dangit

# zsh completion for dangit

_dangit() {
  local context state line
  typeset -A opt_args

  local -a commands
  commands=(
    'scan:Print a report of repos needing attention'
    'resolve:Commit, pull --rebase, and push flagged repos'
    'theme:Manage the active theme (import)'
    'version:Print build version information'
    'help:Show help'
  )

  local -a common_flags
  common_flags=(
    '-t[Per-repo remote-check timeout in seconds]:seconds:'
    '--timeout-secs[Per-repo remote-check timeout in seconds]:seconds:'
    '--no-network[Skip remote checks]'
    '--json[Machine-readable output]'
    '--plain[Force a plain report even on a TTY]'
    '--no-color[Disable colored output]'
    '--no-alt-screen[Render the browser inline]'
    '--theme-file[Theme config path]:file:_files'
    '--help[Show help]'
  )

  if (( CURRENT == 2 )); then
    _describe -t commands 'dangit command' commands
    _arguments $common_flags '*:directory:_files -/'
    return
  fi

  case "${words[2]}" in
    scan)
      _arguments $common_flags '*:directory:_files -/'
      ;;
    resolve)
      _arguments \
        '-y[Execute (default: dry run)]' \
        '--yes[Execute (default: dry run)]' \
        '-m[Commit message]:message:' \
        '--message[Commit message]:message:' \
        '--timeout-secs[Per-repo remote-check timeout in seconds]:seconds:' \
        '--no-color[Disable colored output]' \
        '--theme-file[Theme config path]:file:_files' \
        '*:directory:_files -/'
      ;;
    theme)
      _values 'theme command' 'import[Replace the active theme with a .theme file]'
      ;;
    help)
      _describe -t commands 'dangit command' commands
      ;;
    *)
      _arguments $common_flags '*:directory:_files -/'
      ;;
  esac
}

_dangit "$@"
