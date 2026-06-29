# Contributing

Use the repo's existing Go style: small packages, explicit command contracts,
and focused tests around behavior that touches git state or external commands.

Before opening a PR, run:

```sh
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
```

When adding user-facing commands or flags, update:

- `internal/cli/`
- `docs/non-interactive.md`
- `README.md`
- `man/dangit.1`
- `completions/dangit.{bash,zsh,fish}`

## Tests

Tests build real git repositories in temp dirs (a bare repo as `origin` plus
clones in known states) and drive the engine against them. They run fully
offline and require no network, credentials, or global git config — identity and
isolation are injected via the environment. Never push to a real remote from a
test. See `internal/scan/scan_test.go` for the fixture pattern.

## Shared TUI stack & drift guards

dangit shares its terminal-UI primitives with [passage](https://github.com/0xbenc/passage)
and [ssherpa](https://github.com/0xbenc/ssherpa) through four semver-pinned modules:
`termtheme` (roles + `.theme`), `termnav` (fuzzy matcher + match highlighting),
`termchrome` (footer/box/kvrow + glyphs/spinner), and `termintro` (the startup
animation). Keep them aligned — the `tui-conformance` CI job enforces it:

- **Footers** go through `termchrome.Footer([]termchrome.KeyHint{...})` — never a
  hand-built multi-space separator (CI greps for `  /  `).
- **Spinners** use `termchrome.ResolveGlyphs` — never inline frame runes (CI greps
  for the braille frames and `[]rune{'|',...}`).
- **No golden-update flag** — expectations are inline string literals; pixel changes
  are hand-edited.
- **No `replace`** in the released `go.mod`; pin tagged shared-module versions. When a
  shared module changes, bump dangit (and the sibling apps) to the new tag.
- The `termintro` intro plays on an interactive TTY only, once per version
  (`internal/state/intro.json`); `--intro`/`--no-intro` + `DANGIT_INTRO_ALWAYS`/
  `DANGIT_NO_INTRO` control it.

## Safety

`resolve` is the one mutating, outward-facing path. Keep its guards intact: dry
run by default, explicit `--yes` to execute, a confirmation prompt in the TUI,
no force-push, no auto-merge of conflicts, and a hard refusal when the network
is disabled.
