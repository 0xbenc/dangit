# fish completion for dangit

function __dangit_no_subcommand
    set -l cmd (commandline -opc)
    if test (count $cmd) -eq 1
        return 0
    end
    return 1
end

# Subcommands
complete -c dangit -f -n __dangit_no_subcommand -a scan -d 'Print a report of repos needing attention'
complete -c dangit -f -n __dangit_no_subcommand -a resolve -d 'Commit, pull --rebase, and push flagged repos'
complete -c dangit -f -n __dangit_no_subcommand -a theme -d 'Manage the active theme (import)'
complete -c dangit -f -n __dangit_no_subcommand -a version -d 'Print build version information'
complete -c dangit -f -n __dangit_no_subcommand -a help -d 'Show help'

# theme subcommand
complete -c dangit -f -n '__fish_seen_subcommand_from theme' -a import -d 'Replace the active theme with a .theme file'

# Common flags
complete -c dangit -s t -l timeout-secs -x -d 'Per-repo remote-check timeout in seconds'
complete -c dangit -l no-network -d 'Skip remote checks'
complete -c dangit -l json -d 'Machine-readable output'
complete -c dangit -l plain -d 'Force a plain report even on a TTY'
complete -c dangit -l no-color -d 'Disable colored output'
complete -c dangit -l no-alt-screen -d 'Render the browser inline'
complete -c dangit -l theme-file -r -d 'Theme config path'
complete -c dangit -l help -d 'Show help'

# resolve flags
complete -c dangit -n '__fish_seen_subcommand_from resolve' -s y -l yes -d 'Execute (default: dry run)'
complete -c dangit -n '__fish_seen_subcommand_from resolve' -s m -l message -x -d 'Commit message'
