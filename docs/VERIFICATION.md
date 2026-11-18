# Verification record

Most recent run (Go 1.25.14 on Linux amd64, Chromium 153 via Playwright).

## Passed

- `gofmt -l` clean; `go vet ./...` clean; `go test ./...` and `go test -race ./...` pass (eight packages).
- `go vet ./...` type-checks for windows/amd64, darwin/arm64, darwin/amd64 and linux/arm64 (type-check only; no cross-built binary was executed).
- `npm run frontend:build` produces Tailwind CSS, xterm.js and MathJax assets.
- `node --check` on every frontend script.
- Browser end-to-end (39 checks, 0 console errors): currency text is not rendered as math; MathJax loads only in Research Mode and renders inline and display math; preview scroll survives autosave; preferences persist on the server and apply in a fresh browser origin; delete, trash listing and restore; search arrow-key navigation and dialog semantics; integrity panel; terminal connects and survives hiding the overlay; BibTeX import with provenance, DOI lookup refused while disabled, citation removal, alias resolution; search focus and highlight, Tools chevron alignment, dark dropdown surfaces, Wrap and Format (including math preserved and undo), light-theme standalone document, terminal reattach with replay after a reload. Screenshots reviewed at 1600, 1100, 820 and 390 px wide in both themes.

## Environment caveats

- `proxy.golang.org` and `gitlab.com` were unreachable, so modules could not be resolved. Tests ran in GOPATH mode with Debian-packaged libraries and git clones, with `GODEBUG=httpmuxgo121=0` to match module-mode routing.
- SQLite was provided by `mattn/go-sqlite3` (FTS5) behind a shim that registers the `sqlite` driver name. The shipped `modernc.org/sqlite` driver was not exercised. CI must confirm.
- No `go.sum` was generated.

## Not run

- Tauri desktop build, sidecar lifecycle, installers and updater.
- Windows, WSL2 and macOS runtime checks (ConPTY, file locking, WebView).
- Cross-built binaries.
- Large-vault and large-PDF performance measurements.
- Windows and macOS terminals (the PTY tests ran on Linux only); `scripts/smoke-test.sh`, `scripts/verify-release.sh`.
- Ollama semantic search.

## Earlier records

Earlier passes of this repository recorded static checks only; no compile had been performed. The compile defects found are listed in `docs/AUDIT.md`.
