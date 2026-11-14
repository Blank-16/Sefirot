# Architecture

## Source of truth

`vault/` contains the durable documents and assets. HTML is the interchange format. Markdown-mode notes are rendered to complete HTML documents and retain their original Markdown in a Base64 HTML comment. `index.db` is derived state: deleting it and restarting rebuilds the index from the vault, but revision history and optional embeddings are lost. Existing `./vault` + `./index.db` layouts remain supported when launching from their original directory.

## Write and indexing flow

1. The browser saves through the Go API, or a local CLI writes directly into `./vault`.
2. API writes use `os.Root`; paths are normalized and validated before filesystem access.
3. API writes index immediately. External writes are observed by fsnotify, debounced per path for 150 ms, and indexed.
4. A 30-second reconciliation scan repairs missed watcher events and removes stale index entries.
5. SHA-256 content hashes avoid reparsing unchanged documents.
6. Indexing extracts title, visible text, metadata, tags, headings, wiki-links, and typed relationships. Script and style contents are excluded from FTS text.
7. SQLite triggers keep the external-content FTS5 table synchronized with `notes`.

## SQLite

- Separate reader and writer pools; writer pool is limited to one connection.
- WAL, `busy_timeout=5000`, `synchronous=NORMAL`, and foreign keys are enabled through the DSN.
- FTS5 indexes title and visible text. Search supports prefix terms, quoted phrases, tag/type/folder filters, BM25 ranking, and highlighted snippets.
- `links` stores wiki-link edges; `relationships` stores typed edges. The graph API resolves target slugs to indexed documents.
- `revisions` stores prior HTML on content changes. `agent_runs` stores CLI profile, working directory, timestamps, status, and exit code.
- `embeddings` stores optional local vectors. They are derived and can be regenerated.

## Archive Context protocol

`GET /api/ai/context?path=...` returns a compact JSON context package containing the selected document's metadata, three summary levels, heading outline, backlinks, outgoing linked documents, typed relationships, and a truncation flag. It does not return the entire vault. Agents should use summaries to shortlist documents, then read only the relevant HTML files.

The generated `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, and `CONVENTIONS.md` files describe this workflow to supported CLI agents.

## Search and embeddings

FTS5 works without external services. Optional semantic indexing connects only to an Ollama URL on loopback. Embeddings are generated in batches and stored in SQLite. Hybrid search merges lexical and vector rankings using reciprocal-rank fusion. Vector retrieval currently scans stored vectors; very large vaults may benefit from a dedicated SQLite vector index in a future release.

## Terminal

The Go server launches local CLI processes with a PTY and exposes terminal I/O over a token-protected WebSocket. The browser uses xterm.js. The terminal starts in the configured vault directory. Unix-like platforms use a native PTY; Windows uses ConPTY (Windows 10 1809+) and exposes a WSL profile when `wsl.exe` is available. The operating-system process is not sandboxed to the vault directory.

## Frontend

The frontend is embedded with `embed.FS` and uses Vanilla JavaScript. The frontend can embed generated Tailwind CSS v4 and xterm.js assets using the root `npm run frontend:build` (outputs into `backend/web/`) script. When generated assets are absent, the browser falls back to CDN resources; production desktop builds should run the asset build first to remain offline-capable.

## Product identity

SEFIROT is presented as a personal knowledge archive. The ten-node emblem is an original geometric mark with curved, asymmetric connections. Branding is confined to the interface and documentation; note storage, API routes, indexing, and the vault directory remain unchanged.

## Platform integration

The Go server is the shared application core for browser and desktop modes. `backend/paths.go` resolves the data directory using `SEFIROT_DATA_DIR`, legacy-layout detection, or OS-specific defaults. `process.go` adapts CrossPTY’s shared PTY interface; `process_unix.go` and `process_windows.go` provide platform-specific default shells and shutdown signals. CrossPTY owns PTY creation, resizing, and subprocess-tree cleanup. The Tauri v2 shell in `desktop/` launches the Go server as a sidecar and navigates its native WebView to the server's dynamically selected loopback port.

## Source package boundaries

The Go module is contained entirely in `backend/`, with embedded UI assets in `backend/web/`. It is organized around internal domain packages and focused server files:

- `backend/internal/database`: SQLite schema, persistence mutations, lexical search, graph/context queries, history, embeddings and agent-run records. The schema remains the existing schema; this refactor does not introduce a migration.
- `backend/internal/indexing`: Markdown conversion, HTML extraction, vault path validation, indexing, filesystem watching and reconciliation.
- `backend/internal/semantic`: optional local Ollama embedding and semantic retrieval, depending on the database contract.
- `main.go`: application composition, startup, listener and shutdown lifecycle.
- Backend HTTP files: `assets.go`, `notes.go`, `search.go`, `graph.go`, `history.go`, `system.go`, `transfer.go`, `middleware.go` and `bootstrap.go` group handlers by responsibility.
- `terminal.go` and `process*.go`: terminal sessions and platform-specific process/PTY integration.

Dependency direction is deliberate: HTTP/application composition may depend on domain packages; database and indexing do not depend on HTTP handlers. `internal` keeps these implementation packages private to this module.

## Build-time and runtime dependencies

Go, Rust/Cargo and Node.js/npm are build-time dependencies. Release installers bundle the compiled Go server sidecar, the compiled Tauri desktop shell and static frontend assets. End users should not need development toolchains. Linux runtime packages such as WebKitGTK may still be required by the desktop environment; Windows may require WebView2. The standalone server distribution only requires the platform-compatible Go executable and its bundled web assets.

## Source package boundaries

The Go module is contained entirely in `backend/`, with embedded UI assets in `backend/web/`. It is organized around internal domain packages and focused server files:

- `backend/internal/database`: SQLite schema, persistence mutations, lexical search, graph/context queries, history, embeddings and agent-run records. The schema remains the existing schema; this refactor does not introduce a migration.
- `backend/internal/indexing`: Markdown conversion, HTML extraction, vault path validation, indexing, filesystem watching and reconciliation.
- `backend/internal/semantic`: optional local Ollama embedding and semantic retrieval, depending on the database contract.
- `main.go`: application composition, startup, listener and shutdown lifecycle.
- Backend HTTP files: `assets.go`, `notes.go`, `search.go`, `graph.go`, `history.go`, `system.go`, `transfer.go`, `middleware.go` and `bootstrap.go` group handlers by responsibility.
- `terminal.go` and `process*.go`: terminal sessions and platform-specific process/PTY integration.

Dependency direction is deliberate: HTTP/application composition may depend on domain packages; database and indexing do not depend on HTTP handlers. `internal` keeps these implementation packages private to this module.

## Build-time and runtime dependencies

Go, Rust/Cargo and Node.js/npm are build-time dependencies. Release installers bundle the compiled Go server sidecar, the compiled Tauri desktop shell and static frontend assets. End users should not need development toolchains. Linux runtime packages such as WebKitGTK may still be required by the desktop environment; Windows may require WebView2. The standalone server distribution only requires the platform-compatible Go executable and its bundled web assets.

The browser UI is split into ordered feature scripts under `backend/web/js/` (`core`, `notes`, `ui`, `search`, `events`, `graph`, `history`, `commands`, and `terminal`). They are served from the embedded filesystem and loaded with `defer` in dependency order. This preserves the existing shared UI state and event wiring while isolating feature code into smaller files; converting these transitional classic scripts into strict ES modules is a separate compatibility-sensitive follow-up.

## Research data model

Research papers are stored in `research_papers`, separate from the vault's HTML note files and the existing note FTS index. Authors and keywords are JSON arrays; original PDFs are preserved as vault assets and referenced by a relative `source_file` path. `research_citations` and `research_annotations` use foreign keys with cascade deletion. The `/api/research` API exposes metadata CRUD, citation records, annotations, and PDF attachment. This avoids forcing academic records into the legacy HTML-note schema.

Mathematical source is authored in Markdown using TeX delimiters and rendered in the isolated preview with a locally built MathJax distribution. The source remains authoritative; rendered output is derived. MathJax assets are copied into `backend/web/vendor/mathjax` by `scripts/build-frontend.mjs` from the pinned `mathjax-full` package.
