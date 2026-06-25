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

## Safety

`resolve` is the one mutating, outward-facing path. Keep its guards intact: dry
run by default, explicit `--yes` to execute, a confirmation prompt in the TUI,
no force-push, no auto-merge of conflicts, and a hard refusal when the network
is disabled.
