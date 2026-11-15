package main

import (
	"errors"
	"os"
	"path"
	"sefirot/internal/indexing"
	"strings"
)

func (a *App) seed() error {
	hasHTML := false
	var scan func(string) error
	scan = func(dir string) error {
		entries, err := indexing.ReadDir(a.root, dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			rel := entry.Name()
			if dir != "." {
				rel = path.Join(dir, entry.Name())
			}
			if entry.IsDir() {
				if err := scan(rel); err != nil {
					return err
				}
			} else if strings.EqualFold(path.Ext(entry.Name()), ".html") {
				hasHTML = true
			}
		}
		return nil
	}
	if err := scan("."); err != nil {
		return err
	}
	if hasHTML {
		return nil
	}
	seed := `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Welcome to SEFIROT</title><meta name="description" content="A local-first HTML knowledge workspace with full-text search, wiki-links, and interactive notes."><meta name="vault:type" content="guide"><meta name="vault:tags" content="getting-started, knowledge-management, sefirot">
<style>body{font:17px/1.7 Georgia,"Palatino Linotype",serif;max-width:760px;margin:3rem auto;padding:0 1.25rem;color:#E9E8DF;background:#090B09}h1,h2{color:#F0EFE7;font-weight:400}a{color:#B69B5B}.demo{border:1px solid #34382E;border-radius:2px;padding:1rem;background:#11140F}.demo input{padding:.6rem;background:#090B09;color:#E9E8DF;border:1px solid #34382E;border-radius:2px;width:7rem}.demo button{padding:.6rem .9rem;border-radius:6px;border:0;background:#6366f1;color:white;cursor:pointer}.result{font-size:1.5rem;font-weight:700}</style></head><body>
<h1>Welcome to SEFIROT</h1><p><strong>The Archive</strong> — a local-first space to capture, connect, and retrieve knowledge.</p><p>Your notes are real HTML files stored in this installation’s vault directory, with a SQLite FTS5 index for fast search.</p>
<h2>Wiki-links</h2><p>In Markdown mode, wrap a note name in double square brackets to link to it. Clicking a link to a note that does not exist yet offers to create it.</p>
<h2>HTML and JavaScript are first-class</h2><p>This calculator runs directly inside the isolated preview frame:</p>
<div class="demo"><label>First number <input id="a" type="number" value="12"></label> <label>Second number <input id="b" type="number" value="30"></label> <button id="add">Add</button><p class="result" id="result">42</p></div>
<h2>Quick start</h2><ul><li>Create notes and folders from the left sidebar.</li><li>Use <code>[[Page Name]]</code> for wiki-links in Markdown mode.</li><li>Switch to Raw HTML to author complete documents.</li><li>Search with <kbd>Ctrl</kbd>/<kbd>⌘</kbd> + <kbd>K</kbd>.</li></ul>
<script>document.getElementById('add').addEventListener('click',()=>{const a=Number(document.getElementById('a').value)||0;const b=Number(document.getElementById('b').value)||0;document.getElementById('result').textContent=String(a+b)});</script>
</body></html>`
	f, err := a.root.OpenFile("welcome.html", os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	defer f.Close()
	_, err = f.WriteString(seed)
	return err
}

func (a *App) seedAgentGuides() error {
	guide := strings.Join([]string{
		"# SEFIROT knowledge workflow",
		"",
		"You are working inside the SEFIROT archive directory. HTML files are the source of truth; SQLite and FTS5 are maintained automatically by the running app.",
		"",
		"## Archive Context and retrieval",
		"- Use `/api/ai/context?path=...` as the compact Archive Context protocol. Start with summaries, headings, backlinks, and typed relationships before opening full documents.",
		"- The graph represents actual document relationships; do not invent decorative links.",
		"",
		"## Before changing knowledge",
		"1. Search first: `curl -sG --data-urlencode 'q=your topic' \"$SEFIROT_URL/api/search\"`.",
		"2. For a specific note, fetch compact context: `curl -sG --data-urlencode 'path=Folder/Note.html' \"$SEFIROT_URL/api/ai/context\"`. This returns the note summary, backlinks, and linked-document summaries.",
		"3. Read full HTML only for documents that appear relevant. Prefer summaries and graph relationships for discovery.",
		"",
		"## Creating and connecting documents",
		"- Create valid `.html` documents inside this vault. Use a meaningful `<title>`, one clear `<h1>`, descriptive headings, and concise content.",
		"- Add machine-readable metadata in the HTML head: `<meta name=\"description\" content=\"One-sentence summary.\">`, `<meta name=\"vault:type\" content=\"concept\">`, and `<meta name=\"vault:tags\" content=\"systems, architecture\">`.",
		"- Add links to related notes using anchors with `data-wikilink`, e.g. `<a href='#' class='wiki-link' data-wikilink='Architecture' data-relation='depends-on'>Architecture</a>`. Markdown-mode notes may use `[[Architecture]]`.",
		"- Supported relationship labels include related-to, depends-on, implements, extends, contradicts, references, and parent-of. Use only labels that accurately describe the connection.",
		"- Link only genuinely related concepts. Do not create decorative or redundant links.",
		"- When extending an existing topic, update the existing document instead of creating near-duplicates.",
		"- After writing files, query `/api/search`, `/api/graph`, and `/api/ai/context?path=...` to verify indexing, metadata, and links. The watcher indexes changes after a short debounce and periodically reconciles missed events.",
		"- Use `tag:systems`, `type:concept`, and `folder:Engineering` search filters. `/api/semantic/search` is available when local Ollama embeddings are enabled; `/api/hybrid/search` combines lexical and semantic rankings.",
		"- Never edit `index.db` directly. Never write outside the current vault directory.",
		"",
		"## Output quality",
		"- Put a concise summary in the opening paragraph (the app extracts the first ~42 visible words automatically).",
		"- Use stable, descriptive filenames. Keep nested folders when they represent a meaningful category.",
		"- Preserve existing content and links unless the task explicitly asks to change them.",
	}, "\n")
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md", "CONVENTIONS.md"} {
		f, err := a.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		_, werr := f.WriteString(guide)
		cerr := f.Close()
		if werr != nil {
			return werr
		}
		if cerr != nil {
			return cerr
		}
	}
	return nil
}
