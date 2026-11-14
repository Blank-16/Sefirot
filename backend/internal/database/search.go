package database

import (
	"context"
	"sefirot/internal/mathsearch"
	"strings"
	"unicode"
)

func splitQuotedTerms(raw string) []string {
	var terms []string
	var b strings.Builder
	quoted := false
	flush := func() {
		if b.Len() > 0 {
			terms = append(terms, b.String())
			b.Reset()
		}
	}
	for _, r := range strings.TrimSpace(raw) {
		if r == '"' {
			quoted = !quoted
			b.WriteRune(r)
			continue
		}
		if unicode.IsSpace(r) && !quoted {
			flush()
		} else {
			b.WriteRune(r)
		}
	}
	flush()
	return terms
}

func (d *Database) Search(ctx context.Context, query string) ([]SearchResult, error) {
	return d.SearchFiltered(ctx, query, "", "", "")
}

func (d *Database) SearchFiltered(ctx context.Context, query, tag, docType, folder string) ([]SearchResult, error) {
	terms := splitQuotedTerms(query)
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		if strings.HasPrefix(term, `"`) {
			inner := strings.Trim(term, `"`)
			if inner != "" {
				quoted = append(quoted, `"`+strings.ReplaceAll(inner, `"`, `""`)+`"`)
			}
		} else {
			term = strings.ReplaceAll(mathsearch.ExpandTerm(term), `"`, `""`)
			quoted = append(quoted, `"`+term+`"*`)
		}
	}
	where := []string{"1=1"}
	args := []any{}
	if len(quoted) > 0 {
		where = append(where, "notes_fts MATCH ?")
		args = append(args, strings.Join(quoted, " AND "))
	}
	if tag != "" {
		where = append(where, "EXISTS(SELECT 1 FROM note_tags t WHERE t.path=n.path AND t.tag=?)")
		args = append(args, strings.ToLower(tag))
	}
	if docType != "" {
		where = append(where, "n.doc_type=?")
		args = append(args, docType)
	}
	if folder != "" {
		folder = strings.Trim(folder, "/")
		where = append(where, "(n.path LIKE ? ESCAPE '\\')")
		args = append(args, strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(folder, "\\", "\\\\"), "%", "\\%"), "_", "\\_")+"/%")
	}
	selectSnippet := `snippet(notes_fts,1,'<mark>','</mark>','…',24)`
	from := `notes n JOIN notes_fts ON notes_fts.rowid=n.id`
	order := `bm25(notes_fts,5.0,1.0,0.5)`
	if len(quoted) == 0 {
		selectSnippet = `substr(n.plain_text,1,180)`
		from = `notes n`
		order = `n.updated_at DESC`
	}
	querySQL := `SELECT n.path,n.title,` + selectSnippet + `,n.updated_at FROM ` + from + ` WHERE ` + strings.Join(where, " AND ") + ` ORDER BY ` + order + ` LIMIT 50`
	rows, err := d.Reader.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SearchResult{}
	for rows.Next() {
		var v SearchResult
		if err := rows.Scan(&v.Path, &v.Title, &v.Snippet, &v.Updated); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (d *Database) Backlinks(ctx context.Context, notePath string) ([]Backlink, error) {
	idx, err := d.LinkIndex(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := d.Reader.QueryContext(ctx, `SELECT l.source_path,l.target_slug,n.title FROM links l JOIN notes n ON n.path=l.source_path ORDER BY n.title COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Backlink{}
	seen := map[string]bool{}
	for rows.Next() {
		var src, slug, title string
		if err := rows.Scan(&src, &slug, &title); err != nil {
			return nil, err
		}
		if src == notePath || seen[src] {
			continue
		}
		if target, _ := idx.Resolve(slug); target == notePath {
			seen[src] = true
			out = append(out, Backlink{Path: src, Title: title})
		}
	}
	return out, rows.Err()
}

func (d *Database) Titles(ctx context.Context) (map[string]string, error) {
	rows, err := d.Reader.QueryContext(ctx, `SELECT path,title FROM notes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var p, t string
		if err := rows.Scan(&p, &t); err != nil {
			return nil, err
		}
		out[p] = t
	}
	return out, rows.Err()
}
