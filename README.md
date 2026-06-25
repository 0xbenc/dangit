# dangit

[![CI](https://github.com/0xbenc/dangit/actions/workflows/ci.yml/badge.svg)](https://github.com/0xbenc/dangit/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/0xbenc/dangit?sort=semver)](https://github.com/0xbenc/dangit/releases/latest)
[![Go](https://img.shields.io/badge/go-1.26.3-00ADD8.svg)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Find the git repos you forgot about — *dang it.*

`dangit` sweeps the directories beneath a path and flags every Git repository
with uncommitted changes, commits not pushed to its upstream, or commits waiting
to be pulled. It's the "did I forget to commit or push anything?" ritual: run it
over `~/code` and clean up what shows up. It's a standalone Go rewrite of the
Bash Zoo `forgit` helper, built on the same termtheme engine as
[passage](https://github.com/0xbenc/passage) and
[ssherpa](https://github.com/0xbenc/ssherpa).

## Features

- Full-screen terminal browser with live scan progress and a fuzzy-filterable
  list of flagged repos. Remote checks run concurrently, so a big tree scans
  fast.
- Per-repo actions: open a **detail** view (full `git status`), drop into a
  **shell** there, open it in your **`$EDITOR`**, **copy** its path, or
  **resolve** it.
- **resolve** brings a repo to a clean, synced state — auto-commit, `pull
  --rebase`, push — behind a confirmation prompt. It never force-pushes and never
  auto-merges a conflict.
- Plain-text report and a stable `--json` envelope for scripts and CI, with
  meaningful exit codes (`0` clean · `1` needs attention · `2` usage error).
- Theme-aware via [termtheme](https://github.com/0xbenc/termtheme); themes
  interchange with passage and ssherpa. `NO_COLOR` respected.
- GoReleaser archives, Linux packages, and Homebrew cask publishing.

## Install

### Homebrew cask

```sh
brew install --cask 0xbenc/tap/dangit
```

Or:

```sh
brew tap 0xbenc/tap
brew install --cask dangit
```

### Release artifacts

Download the latest macOS or Linux artifact from
[GitHub Releases](https://github.com/0xbenc/dangit/releases/latest).

Archives are published for `darwin_amd64`, `darwin_arm64`, `linux_amd64`, and
`linux_arm64`:

```sh
tar -xzf dangit_VERSION_OS_ARCH.tar.gz
sudo install -m 0755 dangit /usr/local/bin/dangit
```

Linux packages are also published as `.deb` and `.rpm`.

### From source

Requires Go 1.26.3 or newer.

```sh
git clone https://github.com/0xbenc/dangit.git
cd dangit
go build -trimpath -o dangit ./cmd/dangit
sudo install -m 0755 dangit /usr/local/bin/dangit
```

## Runtime Requirements

- `git` on `PATH`. Nothing else.

## Quick Use

```sh
dangit                 # browse repos under the current directory
dangit ~/code          # browse a specific path
dangit scan ~/code     # plain report (never interactive)
dangit scan --json ~/code   # JSON for scripts
dangit resolve ~/code        # show what resolve would do (dry run)
dangit resolve ~/code --yes  # commit + pull --rebase + push each flagged repo
```

In the browser:

| Key | Action |
| --- | --- |
| `↑`/`↓`, `j`/`k` | Move |
| `/` | Filter (Esc clears, Enter applies) |
| `Enter` | Detail view (full `git status`) |
| `s` | Open a shell in the repo |
| `e` | Open the repo in `$VISUAL`/`$EDITOR` |
| `y` | Copy the repo path |
| `R` | Resolve (with confirmation) |
| `r` | Re-scan |
| `q`, `Esc` | Quit |

## Theme

`dangit` reads `~/.config/dangit/theme.conf` (override with `DANGIT_THEME_FILE`
or `--theme-file`). To adopt a theme exported from a sibling app:

```sh
dangit theme import my.theme
```

This replaces the active theme and backs up the previous one. Roles `dangit`
does not paint are preserved, so the file round-trips with passage and ssherpa.

## Environment

| Variable | Effect |
| --- | --- |
| `DANGIT_TIMEOUT_SECS` | Default per-repo remote-check timeout (seconds, default 10) |
| `DANGIT_NO_NETWORK` | Skip remote checks; behind shows as `stale` |
| `DANGIT_NO_COLOR`, `NO_COLOR` | Disable color |
| `DANGIT_THEME_FILE` | Theme config path |

See [docs/non-interactive.md](docs/non-interactive.md) for the full scripting
reference.

## License

[MIT](LICENSE)
