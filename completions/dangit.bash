# bash completion for dangit

_dangit()
{
    local cur prev words cword
    _init_completion || return

    local commands="scan resolve theme version help"
    local common_flags="--timeout-secs --no-network --json --plain --no-color --no-alt-screen --theme-file --help"

    if [[ $cword -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "$commands $common_flags" -- "$cur") )
        if [[ "$cur" != -* ]]; then
            COMPREPLY+=( $(compgen -d -- "$cur") )
        fi
        return
    fi

    case "${words[1]}" in
        scan)
            COMPREPLY=( $(compgen -W "--timeout-secs --no-network --json --no-color --theme-file --help" -- "$cur") )
            COMPREPLY+=( $(compgen -d -- "$cur") )
            return
            ;;
        resolve)
            COMPREPLY=( $(compgen -W "--yes --message --timeout-secs --no-color --theme-file --help" -- "$cur") )
            COMPREPLY+=( $(compgen -d -- "$cur") )
            return
            ;;
        theme)
            COMPREPLY=( $(compgen -W "import --theme-file --help" -- "$cur") )
            return
            ;;
        help)
            COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
            return
            ;;
        *)
            COMPREPLY=( $(compgen -W "$common_flags" -- "$cur") )
            COMPREPLY+=( $(compgen -d -- "$cur") )
            return
            ;;
    esac
}

complete -F _dangit dangit
