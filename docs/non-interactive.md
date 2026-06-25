# dangit non-interactive reference

`dangit` is a TUI first, but every capability is reachable without a terminal so
it drops cleanly into scripts, cron, and CI.

## Modes

| Invocation | Behavior |
| --- | --- |
| `dangit [PATH]` on a TTY | Interactive browser |
| `dangit [PATH]` piped / non-TTY | Plain text report |
| `dangit [PATH] --plain` | Plain text report (even on a TTY) |
| `dangit scan [PATH]` | Plain text report; never interactive |
| `dangit scan [PATH] --json` | JSON envelope |
| `dangit resolve [PATH]` | Resolve plan (dry run) |
| `dangit resolve [PATH] --yes` | Execute resolve |

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | No repository needs attention |
| `1` | One or more repositories need attention (or a resolve step failed) |
| `2` | Usage error (bad flag, bad timeout, PATH is not a directory, resolve refused offline) |

This makes `dangit scan` a natural pre-commit / pre-logout guard:

```sh
dangit scan ~/code || echo "You have unfinished git work above."
```

## JSON envelope

```sh
dangit scan --json ~/code
```

```json
{
  "root": "code (cwd)",
  "summary": {
    "total": 12,
    "flagged": 2,
    "changes": 1,
    "ahead": 1,
    "ahead_no_upstream": 0,
    "behind": 1,
    "behind_unknown": 0,
    "behind_stale": 0
  },
  "repos": [
    {
      "path": "api",
      "abs_path": "/home/me/code/api",
      "branch": "main",
      "changes": true,
      "ahead": "0",
      "behind": "0"
    }
  ]
}
```

`ahead` is `"0"`, a decimal count, or `"no-upstream"`. `behind` is `"0"`, a
decimal count, `"unknown"` (the remote advanced but isn't fetched locally), or
`"stale"` (the remote could not be reached). Only flagged repos appear in
`repos`; `summary.total` counts every repository scanned.

## resolve

```sh
dangit resolve ~/code            # dry run: prints what it would do
dangit resolve ~/code --yes      # commit + pull --rebase + push each flagged repo
dangit resolve ~/code --yes -m "wip"   # custom commit message
```

`resolve` requires the network and refuses under `--no-network` /
`DANGIT_NO_NETWORK`. It commits any uncommitted changes (auto-generated message
by default), rebase-pulls so local commits replay on top, then pushes. On a
rebase conflict it aborts cleanly, leaves the new commit in place, and reports
the repo for manual resolution. It never force-pushes and never invents a
remote.

## Environment

| Variable | Effect |
| --- | --- |
| `DANGIT_TIMEOUT_SECS` | Default per-repo remote-check timeout (seconds) |
| `DANGIT_NO_NETWORK` | Skip remote checks when truthy |
| `DANGIT_NO_COLOR`, `NO_COLOR` | Disable color |
| `DANGIT_THEME_FILE` | Theme config path |

CLI flags override environment variables, which override built-in defaults.
