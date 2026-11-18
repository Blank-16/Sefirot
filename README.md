# SEFIROT — The Archive

A local-first personal knowledge archive with an integrated CLI terminal, a navigable knowledge graph, SQLite FTS5 search, document history, and optional local semantic search. The vault's `.html` files are the source of truth. SQLite is a rebuildable index and cache.

### Scientific authoring

SEFIROT supports source-first scientific documents in the existing vault: research-paper, literature-review, mathematical-notes, and lab-report templates; a searchable TeX symbol palette; inline/display equations; AMS equation numbering and references; local custom macros; and MathJax rendering in the isolated preview. See [Scientific Authoring](docs/SCIENTIFIC_AUTHORING.md) for supported syntax and current limitations.

## Requirements

- Go 1.25 or newer
- Node.js 20.19+/22.12+ and npm only when building local frontend assets; otherwise the UI falls back to CDN assets
- Optional CLI tools on the host `PATH`: `claude`, `codex`, `gemini`, `aider`, and `agy`
- Optional [Ollama](https://ollama.com/) for local semantic embeddings
- For the native desktop shell: Rust stable, Node.js 20.19+/22.12+, and the platform prerequisites documented in [`desktop/README.md`](desktop/README.md)

## Run

```sh
# Optional but recommended: vendor frontend dependencies for offline use
npm install
npm run frontend:build

cd backend
go mod tidy
go run .

# Optional: build a standalone executable (still inside backend/)
go build -trimpath -o ../sefirot .
```

If the frontend asset build has not been run, the app falls back to Tailwind and xterm CDN assets and therefore needs internet access. The generated local CSS and terminal assets under `backend/web/` are embedded in the Go executable when present at build time.

Open the URL printed at startup. SEFIROT binds to loopback only; if port 8080 is occupied, it selects an available port automatically.

## Repository layout

- `backend/`: Go module, HTTP server, domain packages, tests, and embedded frontend assets in `backend/web/`.
- `desktop/`: Tauri desktop shell and platform capabilities.
- `scripts/`: frontend, sidecar, desktop, and runtime-smoke build scripts.
- `docs/`: architecture, security, roadmap, and verification notes.
- `vault/`: shipped starter note; user data is stored in the configured application data directory.

Run Go commands from `backend/` (or use the root `Makefile`). Frontend build commands remain at the repository root.

## Main features

- Markdown and raw HTML authoring; Markdown is rendered by Goldmark and original Markdown source is embedded in an HTML comment for round-trip editing.
- Scientific notation authoring with a TeX insertion palette and locally bundled MathJax TeX/MathML rendering (after the frontend asset build).
- Real `.html` files and local assets stored in a configurable vault directory. Existing installations with `./vault` keep using that location automatically.
- SQLite WAL with separate reader and writer pools, FTS5 triggers, typed relationships, tags, content hashes, and document revisions.
- Automatic recursive filesystem indexing with per-path debounce and incremental 30-second reconciliation. Unchanged files are skipped by content hash.
- Search operators: `tag:systems`, `type:concept`, and `folder:Engineering`, optionally combined with text terms.
- Three deterministic summary levels (short graph label, medium note summary, and detailed Archive Context), extracted heading outline, and document metadata parsed from HTML `<meta>` tags:

  ```html
  <meta name="description" content="A concise summary of this document." />
  <meta name="vault:type" content="concept" />
  <meta name="vault:tags" content="systems, architecture" />
  ```

- Wiki-links via `[[Note Name]]` or HTML anchors with `data-wikilink`.
- Typed graph relationships via `data-relation`, for example:

  ```html
  <a href="#" data-wikilink="Consensus" data-relation="depends-on">Consensus</a>
  ```

  Supported examples: `related-to`, `depends-on`, `implements`, `extends`, `contradicts`, `references`, and `parent-of`.

- Global and local graph modes, search, pan, zoom, draggable nodes, relationship styling, summaries, and tags.
- Home dashboard, recently updated notes, multiple document tabs, command palette (`Ctrl/Cmd + P`), quick switcher (`Ctrl/Cmd + O`), and global search (`Ctrl/Cmd + K`).
- Document version history and restore from the right-hand properties panel. Optimistic content-hash checks detect edits made by external agents and prevent silent overwrite; the user can reload or explicitly overwrite.
- ZIP vault export and safe import. Import never overwrites existing files, rejects path traversal and symlinks, and re-indexes imported HTML.
- Archive Context protocol: compact summaries, tags, backlinks, outgoing documents, and typed relationship labels.
- CLI terminal using CrossPTY (Unix PTY / Windows ConPTY) with WebSocket for the local shell, Windows WSL (`wsl.exe` when installed), Claude Code, Codex CLI, Gemini CLI, Aider, and an `agy` executable on `PATH`. Up to four local sessions can run concurrently; profile, working directory, duration, exit status, and exit code are stored in an agent activity history.
- The SEFIROT identity uses an original ten-node emblem with curved connections. The existing warm off-white light theme and pure-black dark theme are retained unchanged.

## Cross-platform storage

For a fresh install in browser/server mode, SEFIROT uses an OS-appropriate application data directory:

- Windows: `%LOCALAPPDATA%\SEFIROT`
- macOS: `~/Library/Application Support/SEFIROT`
- Linux and WSL: `$XDG_DATA_HOME/sefirot`, or `~/.local/share/sefirot`

The `vault/` directory and `index.db` live inside that directory. To preserve existing projects, if SEFIROT is launched from a directory with a populated `./vault` (or an existing `./index.db`) and no explicit data paths are configured, it continues using that legacy layout (`./vault` plus `./index.db`). A fresh source archive containing only the shipped `vault/welcome.html` seed uses the OS data directory instead. The HTML note format, API routes, and SQLite schema are unchanged. Keep the database on the local filesystem and do not run two SEFIROT instances against the same vault at once. WSL users should prefer the WSL Linux filesystem over `/mnt/c` for the active vault; the watcher has a periodic reconciliation pass for missed events.

Path overrides:

```sh
# Use an explicit data directory containing vault/ and index.db
sefirot --data-dir "/path/to/sefirot-data"

# Use an existing vault with a separate database location
sefirot --vault-dir "/path/to/existing-vault" --db-path "/path/to/index.db"

# Select an available loopback port (useful for desktop wrappers)
sefirot --port 0
```

Equivalent environment variables: `SEFIROT_DATA_DIR`, `SEFIROT_VAULT_DIR`, and `SEFIROT_DB_PATH`. The legacy `./vault` layout takes precedence only when none of these overrides are set.

## Native desktop application

The `desktop/` directory contains a Tauri v2 shell. It starts the same Go server as a bundled sidecar and loads the existing web UI in a native WebView. It does not fork the data model or duplicate the backend. Build instructions and platform prerequisites are in [`desktop/README.md`](desktop/README.md).

The desktop wrapper uses Tauri’s app-local data directory by default; set `SEFIROT_DATA_DIR` before launching it to point at an existing legacy data directory or a custom location. The desktop wrapper is currently a first implementation scaffold and has not yet been built or runtime-tested on all target operating systems. Do not treat it as a production installer until the native CI/runtime checks are green.

## Optional local semantic search

The application never sends vault text to a hosted embedding API. Semantic indexing uses Ollama on loopback only.

Install Ollama, then run:

```sh
ollama pull nomic-embed-text
ollama serve
```

Start SEFIROT in another terminal. It detects the model and incrementally embeds documents in the background. The search modal offers:

- **Full text** — SQLite FTS5 lexical retrieval.
- **Semantic** — local Ollama embedding similarity.
- **Hybrid** — reciprocal-rank fusion of lexical and semantic results; falls back to whichever source is available.

Optional environment variables:

```sh
SEFIROT_OLLAMA_URL=http://127.0.0.1:11434
SEFIROT_EMBED_MODEL=nomic-embed-text
```

Only loopback Ollama URLs are accepted. The legacy `HTML_VAULT_OLLAMA_URL` and `HTML_VAULT_EMBED_MODEL` names remain supported as fallbacks. Semantic search is optional; full-text search works without Ollama.

## CLI agent workflow

At startup the app creates `vault/AGENTS.md`, `vault/CLAUDE.md`, `vault/GEMINI.md`, and `vault/CONVENTIONS.md` if they do not already exist. Agents should search before creating notes, retrieve `/api/ai/context` for compact linked context, preserve source documents, use metadata and meaningful typed wiki-links, and verify their work through the API.

Example requests:

```sh
curl -sG --data-urlencode 'q=sqlite indexing' http://127.0.0.1:8080/api/search
curl -sG --data-urlencode 'q=tag:systems type:concept' http://127.0.0.1:8080/api/search
curl -sG --data-urlencode 'path=Engineering/Database.html' http://127.0.0.1:8080/api/ai/context
curl -s http://127.0.0.1:8080/api/graph
curl -s http://127.0.0.1:8080/api/health
curl -s http://127.0.0.1:8080/api/semantic/status
```

## API additions

| Endpoint                                                     | Purpose                                                                                          |
| ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------ |
| `GET /api/graph`                                             | Graph nodes and typed edges                                                                      |
| `GET /api/research`                                          | List research records                                                                            |
| `POST /api/research`                                         | Create a research record                                                                         |
| `GET /api/research/{id}`                                     | Read paper metadata, citations and annotations                                                   |
| `PUT /api/research/{id}`                                     | Update paper metadata                                                                            |
| `DELETE /api/research/{id}`                                  | Delete a research record and its dependent records                                               |
| `POST /api/research/{id}/pdf`                                | Attach an original PDF (100 MiB maximum)                                                         |
| `GET/POST /api/research/{id}/citations`                      | List or add citation records                                                                     |
| `GET/POST /api/research/{id}/annotations`                    | List or add page annotations                                                                     |
| `GET /api/ai/context?path=...`                               | Compact Archive Context for a document                                                           |
| `GET /api/ai/search?q=...`                                   | Search results with agent instructions                                                           |
| `GET /api/tags`                                              | Tag counts                                                                                       |
| `GET /api/recent`                                            | Recently updated documents                                                                       |
| `GET /api/agents/runs?limit=20`                              | Recent local CLI agent sessions and exit status                                                  |
| `GET /api/notes/revisions?path=...`                          | Revision history                                                                                 |
| `POST /api/notes/restore`                                    | Restore a revision                                                                               |
| `GET /api/semantic/status`                                   | Local embedding availability and index progress                                                  |
| `GET /api/semantic/search?q=...`                             | Local vector search                                                                              |
| `GET /api/hybrid/search?q=...`                               | Combined lexical and semantic search                                                             |
| `GET /api/health`                                            | Index/database health summary                                                                    |
| `GET /api/export`                                            | Download a ZIP backup of the vault                                                               |
| `POST /api/import`                                           | Import a ZIP; validated up front, never overwrites, rolls back on failure                        |
| `DELETE /api/research/{id}/citations/{cid}`                  | Remove a citation record                                                                         |
| `DELETE /api/research/{id}/annotations/{aid}`                | Remove an annotation                                                                             |
| `GET /api/trash`                                             | List deleted notes and research files (30-day retention)                                         |
| `POST /api/trash/restore`                                    | Restore a trashed item; refuses to overwrite, accepts an alternative `path`                      |
| `DELETE /api/trash?id=...`                                   | Permanently delete one trashed item                                                              |
| `GET/PUT /api/settings`                                      | Interface preferences stored next to the index (theme, tabs, Research Mode documents, macros)    |
| `GET /api/resolve?target=`                                   | Resolve a wiki-link target by path, file name, title or alias; returns candidates when ambiguous |
| `POST /api/research/import`                                  | Import BibTeX or RIS (format auto-detected); duplicates are matched by DOI                       |
| `GET /api/research/export?format=bibtex\|ris&ids=`           | Export references; unedited imports are written back verbatim                                    |
| `GET /api/research/bibliography?style=apa\|ieee\|plain&ids=` | Formatted reference list                                                                         |
| `POST /api/research/lookup-doi`                              | Fetch metadata from Crossref; refused until DOI lookup is enabled in the research settings       |
| `GET /api/integrity`                                         | Broken links, ambiguous links (same note name in several folders) and unlinked notes             |

## Notes, links and search

- Markdown notes accept YAML front matter at the top: `aliases`, `tags`, `type`, `title` and `description`. Front matter stays in the editable source and is not rendered into the page. Aliases and titles resolve wiki-links, so `[[QM]]` or `[[Quantum Mechanics]]` finds `2024-quantum.md` without matching its file name.
- Link resolution order: full path, file name, title, alias. If several notes match at the same step the link is reported as ambiguous under "Needs attention" and nothing is guessed.
- Math is protected from Markdown: `$...$`, `$$...$$`, `\(...\)` and `\[...\]` are passed to MathJax exactly as typed, including `_`, `*` and `\`. A `$` followed by a space or digit, as in "$5 and $10", is plain text. MathJax loads only for notes in Research Mode.
- Search understands notation. `∇`, `\nabla`, `nabla` and `gradient` all find notes that use that symbol in any form; Greek letters match their LaTeX names.

## References and provenance

Papers carry an origin: entered manually, imported from BibTeX or RIS, or retrieved from Crossref. Imports keep the original record verbatim and show it under "Original record". Editing the bibliographic fields of an imported paper marks it as edited; the original stays available, and unedited imports export byte-for-byte. Citations and annotations carry their own origin (`manual`, `extracted`, `ai`), set by the server, never by a client.

DOI lookup is off by default. When enabled in the research library it sends only the DOI to `api.crossref.org`.

## Terminal

Terminal sessions are owned by the server. Hiding the overlay, closing the tab or reloading the page leaves the session running; reopening the terminal reattaches and replays the last 256 KiB of output. A session with no browser attached is ended after 10 minutes, and "End session" ends it immediately. `GET /api/terminal/sessions` lists live sessions (token required). The WSL profile is offered only on Windows; on Linux, including inside WSL, the default is the plain shell (`$SHELL`, else bash). The xterm.js library is bundled under `/vendor`; if that copy is missing the page loads it from `cdn.jsdelivr.net` and says so in the terminal status line. Both `127.0.0.1` and `localhost` are accepted as the host and origin of the WebSocket.

## Source editor

The source editor has a Wrap toggle (long lines wrap or scroll; the choice is remembered in the browser and in the server settings) and a Format button. Format uses a bundled copy of Prettier, loaded on first use. Math spans (`$...$`, `$$...$$`, `\(...\)`, `\[...\]`) and YAML front matter are set aside before formatting and restored byte for byte; if any span would change, nothing is applied. The result is inserted as one undoable edit, so Ctrl+Z restores the original.

## One process per data directory

SEFIROT writes `.sefirot.lock` (process id, host and a heartbeat) in the data directory and refuses to start while a live process holds it. The check works across Windows and WSL because both read the same file. A lock whose heartbeat is older than 20 seconds is treated as left by a crash. A warning is logged when the data directory is a Windows drive mounted in WSL, a WSL path opened from Windows, or a network share, since SQLite's write-ahead log is unreliable there.

Before any schema migration touches a database that holds data, a consistent copy is written next to it as `index.db.pre-vN-<time>.bak` (the three most recent are kept). Schema versions are recorded in `schema_migrations`.

## Recovery model

- Deleting a note moves it to `vault/.sefirot/trash` for 30 days (Tools > Trash). Its revision history is kept for the same period.
- Revision snapshots are coalesced: a new snapshot of the previous content is stored only when the newest one is at least five minutes old, so 50 revisions span real editing sessions instead of 40 seconds of typing. Restoring a revision first snapshots the current version.
- Research papers, citations and annotations are stored as `research/<id>/record.json` inside the vault and indexed into SQLite. They are included in exports and are restored automatically if `index.db` is deleted. Deleting a paper trashes its record and PDF.
- Import validates every archive entry before writing anything, counts actual decompressed bytes (not header sizes), and removes files it created if it fails part-way. Existing files are never overwritten; identical research ids from another vault are skipped and reported.

## Storage and recovery

- `vault/` under the configured data directory: source-of-truth documents and assets (or the existing `./vault` in legacy mode)
- `index.db` alongside the vault: rebuildable SQLite index, revision store, summary cache, and local embeddings

Stop the application before deleting `index.db`; the next startup rebuilds the index from the vault. The vault remains usable in any browser even without SEFIROT, though application-only wiki-link navigation and search will not be available.

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md), [`docs/SECURITY.md`](docs/SECURITY.md), and [`docs/ROADMAP.md`](docs/ROADMAP.md) for the architecture, security assumptions, and explicitly deferred ideas.

## Security model

- The HTTP server binds to `127.0.0.1` on the configured or selected port and rejects non-local Host/Origin combinations.
- Vault reads and writes use `os.Root` and validated relative paths. Saves stage the new content in `vault/.sefirot/tmp`, fsync it, and replace the note with `Root.Rename`, so a crash leaves either the old or the new file. This requires Go 1.25. `vault/.sefirot/` is reserved application state (staging files, trash); it is never indexed, listed, exported or served.
- Note previews run in an iframe sandbox with scripts enabled but without `allow-same-origin`; trusted local HTML/JavaScript can execute in that isolated frame.
- The terminal executes local programs with the same operating-system permissions as the Go process. Unix-like systems use a PTY; Windows uses ConPTY and requires Windows 10 version 1809 or later. This is not a process sandbox; CLI agents may access files outside the vault and may send prompts or files to their own providers. Do not expose the server through a LAN, tunnel, or reverse proxy.
- Terminal creation and WebSocket sessions require a per-process random token. WebSocket origins are checked.
- Ollama embedding calls are restricted to loopback; no note content is sent to cloud embedding services.

## Verification

See [`docs/VERIFICATION.md`](docs/VERIFICATION.md) for the checks run during packaging and the Go toolchain limitation.

Run the automated checks with:

```sh
find backend -name '*.go' -print0 | xargs -0 gofmt -w
(cd backend && go test ./...)
node --check backend/web/app.js
```

The repository includes tests for HTML extraction, metadata, relationship parsing, path validation, Markdown source round-tripping, SQLite FTS/search/filtering, revisions, and vector similarity.

## Developer toolchains versus end-user requirements

Building from source requires Go 1.25+, Node.js/npm for frontend assets, and Rust/Cargo for the Tauri desktop package. These are developer/build dependencies. The intended end-user desktop distribution bundles the compiled Tauri application, Go backend sidecar, and frontend assets; users do not install Go, Rust, or Node.js. Linux desktop packages may depend on the system WebKitGTK runtime, and Windows may need WebView2. See `docs/ARCHITECTURE.md` for package boundaries and distribution details.

## Research workspace and scientific notation

The editor includes a TeX insertion palette and renders TeX/MathML through a locally bundled MathJax distribution after `npm install` and `npm run frontend:build`. Supported authoring includes inline and display math, aligned equations, matrices, common operators, Dirac bra-ket notation, and `mhchem` chemical equations. The underlying TeX source remains in the Markdown source.

The Research workspace stores paper metadata, author lists, abstracts, DOI/publication details, keywords, reading status, personal notes, attached original PDFs, citation records, and page-based annotations in SQLite. PDF uploads are preserved unchanged inside the vault and are limited to 100 MiB. Research records are separate from existing HTML note files, so the existing vault format is not rewritten.

The current research foundation does **not yet** claim full PDF structure extraction, OCR of mathematical notation, bibliography import/export, LaTeX project compilation, scientific plotting, or AI paper analysis. Those are subsequent integration stages; a stored PDF is not the same as a parsed paper. GROBID and Tectonic integrations will be optional local tools, with explicit status and error reporting rather than silently degrading.
