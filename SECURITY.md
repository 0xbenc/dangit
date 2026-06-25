# Security Policy

`dangit` shells out to `git` to inspect repositories and, only on the `resolve`
path, to commit and push on your behalf. It reads no secrets and stores no
credentials.

## Reporting

Please report vulnerabilities privately to:

```text
info@offcourtcreations.com
```

Include reproduction steps, affected versions or commits, and whether the issue
can cause unintended writes or pushes to a remote.

## Supported Versions

Only the latest released version is supported for security fixes.

## Design Notes

- Scanning is read-only. The only mutating command is `resolve`.
- `resolve` is a dry run unless `--yes` is given, and prompts for confirmation in
  the TUI before acting.
- `resolve` never force-pushes, never auto-merges a conflicting rebase (it aborts
  and reports the repo), and never invents a remote to push to.
- Remote checks run with `GIT_TERMINAL_PROMPT=0` and SSH `BatchMode=yes`, so a
  repo with a missing credential fails fast instead of hanging on a prompt.
- `--no-network` / `DANGIT_NO_NETWORK` disables all remote contact and makes
  `resolve` refuse to run.
