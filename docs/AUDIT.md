# Audit log

Findings from the engineering review of the uploaded source tree, with status.

## Fixed

| Area | Finding | Resolution |
|---|---|---|
| Build | Tree did not compile: undefined `summarizeDocument`/`summarizeWords`, `log` not imported in `indexing` and `main`, `rootReadDir` only in `main`, nonexistent `gmhtml.WithNodeRenderers`, unexported field access in `search.go`, undefined `errorString`, duplicate test import | Corrected; `go vet` and `go test` pass |
| Data loss | Note saves truncated the file before writing | Atomic stage + fsync + rename (`internal/vaultfs`) |
| Data loss | Delete removed the file and its revision history | Trash with 30-day retention; revisions survive deletion |
| Data loss | 50-revision cap consumed in under a minute of autosaves | Snapshots coalesced to one per five minutes |
| Integrity | Research data existed only in SQLite and was missing from exports | Vault-backed `record.json`, rebuilt on startup and after import |
| Integrity | Export failed on files over 32 MiB and returned a truncated 200 on error | Built to a temp file first; 128 MiB per file |
| Security | Import trusted ZIP header sizes, buffered in memory, could half-apply | Streamed, pre-validated, byte-counted, rolled back on failure |
| Correctness | `updated_at` was index time | File modification time |
| Correctness | New empty Markdown notes were detected as raw HTML, so Markdown was never rendered | Source-comment pattern accepts an empty payload |
| Correctness | Markdown mangled LaTeX (`_`, `*`, `\`) | Math spans are preserved verbatim by a goldmark parser; currency such as "$5 and $10" is not treated as math |
| Frontend | MathJax delimiters lost their backslashes; `$$` was not a display delimiter | Fixed escaping; `$$...$$` added |
| Frontend | `String.replace` interpreted `$'` in injected script text | Function replacers |
| Frontend | `/visual-system.css` had no route; the redesign stylesheet was never served | Route added |
| Frontend | MathJax fonts blocked by CORS in the sandboxed preview | `/vendor/` GETs are exempt from the Origin check and send CORS headers; API stays closed to opaque origins |
| Frontend | MathJax loaded for every note | Loaded only in Research Mode |
| Frontend | Preview scroll reset on every autosave | Position reported by the preview and restored |
| Frontend | Desktop uses a random port, so `localStorage` preferences were lost every launch | Preferences stored server-side (`/api/settings`) with browser storage as a fast cache |
| Frontend | Clicking outside the terminal killed the running agent | Overlay hides; "End session" stops it explicitly |
| Frontend | Split view unusable on narrow screens; edit pane did not fill height | Split hidden below 800 px; pane layout fixed |
| Routing | Unknown routes returned the index page with 200 | 404, JSON under `/api/` |
| Accessibility | Dialogs had no role, label or focus handling; search and palette had no arrow-key navigation | Added |
| First run | Seed note linked to a missing note and used an off-brand palette | Rewritten |

| Schema | No migration framework; columns were added with ignored errors | Versioned migrations, transactional, with a pre-migration backup and a forced reindex when derived data changes |
| Links | Wiki-links resolved by file name only; same-named notes silently collided | Path, name, title and alias resolution; ambiguity reported, never guessed |
| Notes | Markdown notes could not carry tags, type or aliases | YAML front matter |
| Search | Notation was invisible to full-text search | `math_text` index column and query expansion |
| Research | No standard bibliography formats; no provenance | BibTeX and RIS import and export, opt-in Crossref lookup, per-record origin, verbatim original records |
| Research UI | Citations and annotations could not be removed | Remove controls, origin badges |
| Concurrency | Two processes (Windows and WSL) could open one live database | Heartbeat lock file and risky-path warnings |
| CI | No vet, race detector or browser checks | Added; browser job uploads screenshots |

## Open

- Page reload still ends terminal sessions (no server-side reattach).
- Ambiguous wiki-links draw no graph edge until the author disambiguates them.
- Research workspace: no in-app PDF reader, annotations are manual page and quote records, no claims/evidence ledger, no equation-level provenance.
- No in-app AI assistant; "summaries" are deterministic word truncations.
- Graph redraws the full SVG on pointer move and has a static layout.
- Tailwind zinc utilities are re-themed through `!important` overrides in `visual-system.css`.
- Polling every 7 seconds walks the vault tree.
- `go.sum` is not committed (CI warns).
- No Python/SymPy execution layer.
- Windows rename-over-open-file behaviour, WSL/Windows shared-database locking, desktop packaging and updates are unverified.

The plan for every open item is in `docs/LIMITATIONS_PLAN.md`.
