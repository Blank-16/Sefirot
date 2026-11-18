# Plan for the remaining known limitations

Ordered by user impact and risk. Each item states the problem, the design, the files involved, how it is verified, and what it depends on. Effort: S under a day, M a few days, L a week or more.

## 0. Prerequisites that block verification (do first)

| Item                 | Action                                                                                                                                                                                        | Effort |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ |
| `go.sum`             | Run `go mod tidy` in `backend/` on a networked machine and commit `go.sum`. CI already warns while it is missing.                                                                             | S      |
| `modernc.org/sqlite` | Let CI run the full suite in module mode. Local runs so far used `mattn/go-sqlite3` behind a shim. Confirm `VACUUM INTO` (migration backups) and FTS5 behave identically.                     | S      |
| Desktop              | Build the Tauri sidecar on all three OS families; confirm `--port 0`, ready file, token handshake, shutdown, and that the new instance lock releases on quit and on crash (stale after 20 s). | M      |
| Windows              | Test `Root.Rename` over a file the watcher has open (retry loop exists, untested); test the instance lock across a Windows build and a WSL build sharing one folder.                          | M      |

## 1. Terminal sessions survive reload (done)

Implemented: server-owned sessions, 256 KiB replay buffer, takeover by a reloaded page, 10-minute idle timeout, session list endpoint. Remaining: Windows ConPTY and macOS verification, and a visible "running sessions" list in the UI when more than one exists.

## 2. Wiki-link and note-identity polish (P1, S)

Done: title and alias resolution, ambiguity reporting. Remaining: when a link is ambiguous the graph draws no edge. Add a "Resolve" action in the Needs attention panel that rewrites the link to the full path (a source edit, previewed first, snapshotted as a revision). Show note titles on tabs instead of file names.
Files: `web/js/maintenance.js`, `web/js/notes.js`, a new `POST /api/notes/relink` in `notes.go`.
Verify: handler test for rewrite and revision snapshot; browser check.

## 3. Research workspace: evidence model (P2, L)

Problem: annotations are manual page and quote strings, and nothing distinguishes source text from interpretation beyond the new `origin` field.

Design:

1. Add `claims` and `evidence` tables (vault-backed in `record.json`): a claim has text, status (proposed, supported, contradicted, retracted) and an `origin`; evidence links a claim to a paper, page, annotation or equation with a quoted span and a confidence field set only by the user.
2. Equation records: LaTeX source, `origin` (typed, extracted, ocr), `ocr_confidence`, page and bounding box, and a `corrected_from` field holding the untouched original whenever a user edits it. Nothing is ever rewritten in place.
3. Cross-reference syntax `[@key]` for citations and `[[eq:name]]` for equations in Markdown, resolved at render time; unresolved keys are reported by `/api/integrity`.
   Files: `internal/database/research.go`, `migrate.go` (versions 4 and 5), `research_store.go`, `web/js/research.js`.
   Verify: migration test from v3, record round-trip, integrity cases for dangling keys.
   Depends on: item 4 for page coordinates.

## 4. In-app PDF reader with visual annotations (P2, L)

Problem: PDFs open in a browser tab; annotations are not anchored.

Design: vendor pdf.js into `web/vendor` (no CDN), render pages in a sandboxed viewer pane served from `/vault-assets`, capture selections as `{page, quads, quote}`, and save as annotations with `origin: manual`. Text extraction for search uses pdf.js text content run in the browser, uploaded as a sidecar `research/<id>/text.json` flagged `origin: extracted`.
Optional: a `--grobid-url` setting enabling structure and reference extraction; results are stored as separate records with `origin: extracted` and never overwrite user fields. External calls are disclosed and off by default, like DOI lookup.
Files: `web/js/pdfviewer.js`, `scripts/build-frontend.mjs`, `research.go`, CSP in `middleware.go` for the viewer worker.
Verify: browser check that selecting text creates an annotation anchored to the page; export includes anchors.
Risk: pdf.js size and worker CSP; parsing happens in the browser sandbox, not in Go.

## 5. AI assistance with provenance (P2, L)

Problem: AI today is external CLI agents plus word-truncation "summaries".

Design: an in-app assistant that only answers from retrieved vault passages. Retrieval combines FTS5 (including `math_text`) and the existing embeddings. Every answer is returned as segments tagged `source` (quoted passage with path, heading or page), `extracted`, `ai` or `user`, rendered with distinct styling. The model backend is Ollama by default (local); any remote provider requires an explicit setting and is disclosed in the UI. Imported documents are wrapped as untrusted data in the prompt and never concatenated with instructions.
Files: new `internal/assistant`, `search.go`, `web/js/assistant.js`.
Verify: golden tests for segment tagging with a fake model; prompt-injection fixture documents; the UI never shows an untagged AI sentence.

## 6. Computational notebooks (P3, L)

Design: a separate worker process (`sefirot-runner`, Python) started on demand, never in-process. Execution is confined with the strongest available mechanism per OS (Linux: bubblewrap or a rootless namespace plus rlimits; macOS: `sandbox-exec`; Windows: a job object with limits and a restricted token), no network, a scratch directory, CPU, memory and wall-clock caps. If no sandbox is available the feature stays disabled and says why. Results are saved beside the note with the code hash and environment lockfile for reproducibility.
Verify: escape tests (network, file reads outside scratch, fork bombs) per platform.
Depends on: item 1 for process supervision patterns.

## 7. Graph (P2, M)

Problem: the SVG is rebuilt on every pointer move and uses a static ring layout.

Design: move to a canvas renderer, run a force layout in a Web Worker with Barnes-Hut approximation, render only visible nodes, and add filters (folder, tag, type, relation) and community colouring computed server-side from the link graph (Louvain over `links` and `relationships`). Edges for typed relations and, optionally, citation links between papers.
Files: `web/js/graph.js`, `graph.go`, `internal/database/graph.go`.
Verify: benchmark with 5 000 generated notes; target 60 fps pan and zoom, first paint under one second.

## 8. Replace polling with change events (P2, S)

Problem: every 7 seconds the client walks the whole vault tree.

Design: the watcher already sees changes. Publish them over `GET /api/events` (Server-Sent Events: path, kind). The client refreshes only what changed and falls back to a 60 s poll if the stream drops. The tree endpoint gains an `If-None-Match` version tag.
Files: `internal/indexing/vault.go`, new `events.go`, `web/js/events.js`, `web/js/core.js`.
Verify: Go test for event delivery; browser check that an external file edit appears without polling.

## 9. Design tokens instead of `!important` overrides (P3, M)

Problem: Tailwind zinc utilities are re-coloured per theme through `!important` rules in `visual-system.css`.

Design: define colour, spacing and type tokens as CSS variables (they exist already), expose them to Tailwind v4 through `@theme`, and replace `zinc-*` and `indigo-*` utility use in `index.html` and `web/js/*.js` with semantic classes (`bg-surface`, `text-muted`, `border-line`). Do it screen by screen behind the screenshot suite, deleting each override as its last user disappears.
Verify: screenshot diff on the 16 existing captures; target zero `!important` outside reduced-motion and print rules.

## 10. Remaining smaller items

| Item                                                      | Fix                                                                                                                  | Effort |
| --------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- | ------ |
| Research id collisions on import                          | Remap ids on import: assign new ids, rewrite `record.json`, rename the directory, rebase `cited_paper_id`            | S      |
| Research Mode is lost when a note is deleted and restored | Store the flag in the note (`<meta name="vault:mode">` or front matter `mode: research`) so it travels with the file | S      |
| Summaries repeat the title and show raw TeX               | Strip the leading heading and math spans before summarising                                                          | S      |
| `Ctrl+O` duplicates `Ctrl+K`                              | Make it a recent-notes switcher                                                                                      | S      |
| Large vaults                                              | Measure indexing and tree time at 10 000 notes; virtualise the sidebar tree if the DOM cost shows                    | M      |
| LaTeX project import and export (`.tex` + `.bib`)         | Export a research note and its bibliography as a `.tex` project; optional Tectonic compile behind a setting          | M      |
| Citation styles                                           | Add Chicago and Vancouver; load CSL styles from the vault if present                                                 | M      |

## Suggested order

1. Prerequisites (section 0), then item 8 and item 10's small fixes.
2. Item 1 (terminal), item 2 (links), item 7 (graph).
3. Items 3 and 4 together (evidence model and PDF reader).
4. Item 5 (assistant), then item 6 (notebooks) once the sandbox spike succeeds on all three operating systems.
5. Item 9 (tokens) in parallel as a low-risk background task.
