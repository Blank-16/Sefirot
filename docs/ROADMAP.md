# Deferred product work

The current application includes local document storage, FTS5, automatic indexing, typed wiki-link graph, summaries and outlines, revision history, CLI terminal sessions, agent-run history, context retrieval, optional local embeddings, hybrid search, and ZIP backup/import.

The following ideas remain separate implementation projects rather than partially implemented features:

- AI-suggested semantic graph edges with accept/reject workflow.
- Automatic source citation/provenance extraction and contradiction detection.
- Document lifecycle states, aliases, and richer frontmatter editing.
- Dedicated graph clustering, bridge-node metrics, tag/folder/date filters, and a minimap.
- Approximate-nearest-neighbor vector index for very large vaults.
- PDF, image OCR, codebase, CSV, and audio transcript indexing.
- Multi-agent task orchestration and explicit per-agent read/write/execute permission profiles.
- Scheduled backups, backup retention, and restore-point management.
- Mobile companion, plugin API, spaced repetition, and knowledge timeline.
- Rich CodeMirror editor features such as syntax highlighting, code folding, and multi-cursor editing.

These should be added behind stable APIs and migrations, with the HTML document vault remaining the source of truth.

## Identity

The product name is SEFIROT (The Archive). The emblem uses ten connected nodes and curved paths as an original visual metaphor for interconnected knowledge. Existing light and dark theme palettes are intentionally preserved.

## Cross-platform rollout status

Implemented in the current branch: OS-specific data directory selection, legacy vault/database path detection, Unix PTY and Windows ConPTY adapters, Windows WSL terminal profile, dynamic loopback port selection, authenticated graceful shutdown, local frontend asset build with CDN fallback, Tauri v2 desktop sidecar scaffold, native CI matrix, and runtime smoke-test scripts.

Still requiring native verification before release: Go 1.25+ test/build on all three desktop OS families, Windows ConPTY interaction and process cleanup, WSL profile working-directory behavior, Tauri desktop build/runtime on Windows/Linux/macOS, and installer upgrade/uninstall behavior. Linux distributions and older Windows builds may require additional platform-specific packaging work.

## Research and scientific authoring completion track

- [x] Separate research-paper metadata records from legacy note files.
- [x] Local paper metadata CRUD, DOI field, author/abstract/keyword metadata, and reading states.
- [x] Preserve original PDF attachments in the local vault with a size limit.
- [x] Store citations and page-based annotations independently of source PDFs.
- [x] Add a TeX insertion palette and local MathJax rendering build integration.
- [ ] Add robust PDF text/layout extraction and structured paper parsing through optional local GROBID.
- [ ] Add equation OCR and confidence-aware review; preserve page/region provenance.
- [ ] Add BibTeX/BibLaTeX/RIS import and export, CSL formatting, and DOI metadata resolution.
- [ ] Add full LaTeX project import/export and optional Tectonic compilation.
- [ ] Add structured paper sections, figures, tables, algorithms, theorem/proof environments, and cross-references.
- [ ] Add scientific diagram and plotting workflows, including CSV datasets and SVG export.
- [ ] Add paper/equation/concept citation graph and source-grounded AI research analysis.
- [ ] Add cross-platform end-to-end tests and format round-trip fixtures for all supported formats.

A checked item means code has been added, not that the overall research feature set is complete. Release acceptance requires all unchecked items to be implemented and verified.

### Scientific authoring foundation — implemented in source

- Research-paper, literature-review, mathematical-notes, and lab-report templates.
- Searchable TeX symbol insertion and common scientific notation snippets.
- MathJax preview configuration with AMS numbering/references and local custom macros.
- See `SCIENTIFIC_AUTHORING.md`; frontend build and browser verification remain outstanding.
