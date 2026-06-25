# Releasing

A release is cut by pushing a `vX.Y.Z` tag, which triggers the goreleaser
**Release** workflow (`.github/workflows/release.yml`). The version is taken from
the tag (`-ldflags -X main.version`), so there is nothing to edit in code.

## One-time setup

Add a `TAP_GITHUB_TOKEN` secret to this repository — a PAT with write access to
`0xbenc/homebrew-tap` (the same token used by `passage` and `ssherpa`).
GoReleaser uses it to update the cask.

## Steps

1. Make sure `main` is green and holds everything to ship.

2. **If `github.com/0xbenc/termtheme` changed, release it first** (see that
   repo's `RELEASING.md`), then bump the pin here:

   ```sh
   go get github.com/0xbenc/termtheme@vX.Y.Z
   go mod tidy && go test ./...
   git commit -am "Bump termtheme to vX.Y.Z"
   ```

   dangit pins termtheme by tag with **no `replace` directive**, so the termtheme
   tag **must already exist on the proxy** before this release builds in CI —
   that is why termtheme is tagged first.

3. Tag and push:

   ```sh
   git tag -a vX.Y.Z -m "..."
   git push origin main
   git push origin vX.Y.Z      # triggers the Release workflow
   ```

If termtheme did not change, skip step 2 entirely.

## Versioning

Semantic versioning. Backward-compatible features are a **minor** bump;
fixes are a **patch** bump.
