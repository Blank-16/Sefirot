# SEFIROT knowledge workflow

You are working inside the SEFIROT archive directory. HTML files are the source of truth; SQLite and FTS5 are maintained automatically by the running app.

## Archive Context and retrieval

- Use `/api/ai/context?path=...` as the compact Archive Context protocol. Start with summaries, headings, backlinks, and typed relationships before opening full documents.
- The graph represents actual document relationships; do not invent decorative links.

## Before changing knowledge

1. Search first: `curl -sG --data-urlencode 'q=your topic' "$SEFIROT_URL/api/search"`.
2. For a specific note, fetch compact context: `curl -sG --data-urlencode 'path=Folder/Note.html' "$SEFIROT_URL/api/ai/context"`. This returns the note summary, backlinks, and linked-document summaries.
3. Read full HTML only for documents that appear relevant. Prefer summaries and graph relationships for discovery.

## Creating and connecting documents

- Create valid `.html` documents inside this vault. Use a meaningful `<title>`, one clear `<h1>`, descriptive headings, and concise content.
- Add machine-readable metadata in the HTML head: `<meta name="description" content="One-sentence summary.">`, `<meta name="vault:type" content="concept">`, and `<meta name="vault:tags" content="systems, architecture">`.
- Add links to related notes using anchors with `data-wikilink`, e.g. `<a href='#' class='wiki-link' data-wikilink='Architecture' data-relation='depends-on'>Architecture</a>`. Markdown-mode notes may use `[[Architecture]]`.
- Supported relationship labels include related-to, depends-on, implements, extends, contradicts, references, and parent-of. Use only labels that accurately describe the connection.
- Link only genuinely related concepts. Do not create decorative or redundant links.
- When extending an existing topic, update the existing document instead of creating near-duplicates.
- After writing files, query `/api/search`, `/api/graph`, and `/api/ai/context?path=...` to verify indexing, metadata, and links. The watcher indexes changes after a short debounce and periodically reconciles missed events.
- Use `tag:systems`, `type:concept`, and `folder:Engineering` search filters. `/api/semantic/search` is available when local Ollama embeddings are enabled; `/api/hybrid/search` combines lexical and semantic rankings.
- Never edit `index.db` directly. Never write outside the current vault directory.

## Output quality

- Put a concise summary in the opening paragraph (the app extracts the first ~42 visible words automatically).
- Use stable, descriptive filenames. Keep nested folders when they represent a meaningful category.
- Preserve existing content and links unless the task explicitly asks to change them.
