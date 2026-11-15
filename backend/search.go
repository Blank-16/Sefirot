package main

import (
	"context"
	"net/http"
	"sefirot/internal/database"
	"sefirot/internal/semantic"
	"sort"
	"strings"
	"time"
	"unicode"
)

func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	query, tag, docType, folder := parseSearchFilters(r.URL.Query().Get("q"))
	if query == "" && tag == "" && docType == "" && folder == "" {
		writeJSON(w, []database.SearchResult{})
		return
	}
	results, err := a.db.SearchFiltered(r.Context(), query, tag, docType, folder)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	writeJSON(w, results)
}

func (a *App) handleAISearch(w http.ResponseWriter, r *http.Request) {
	q, tag, docType, folder := parseSearchFilters(r.URL.Query().Get("q"))
	if q == "" && tag == "" && docType == "" && folder == "" {
		writeJSON(w, map[string]any{"query": q, "results": []database.SearchResult{}})
		return
	}
	results, err := a.db.SearchFiltered(r.Context(), q, tag, docType, folder)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	writeJSON(w, map[string]any{"protocol": "html-vault-context-v1", "query": q, "results": results, "instruction": "Use snippets to shortlist notes. Fetch /api/ai/context for a selected note before opening full documents. Do not treat lexical matches as proof of semantic equivalence."})
}

func (a *App) handleTags(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Reader.QueryContext(r.Context(), `SELECT tag,COUNT(*) FROM note_tags GROUP BY tag ORDER BY tag COLLATE NOCASE`)
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	defer rows.Close()
	type tagCount struct {
		Tag   string `json:"tag"`
		Count int    `json:"count"`
	}
	out := []tagCount{}
	for rows.Next() {
		var v tagCount
		if err := rows.Scan(&v.Tag, &v.Count); err != nil {
			jsonError(w, 500, err)
			return
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, out)
}

func (a *App) handleRecent(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Reader.QueryContext(r.Context(), `SELECT path,title,summary,updated_at FROM notes ORDER BY updated_at DESC LIMIT 12`)
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	defer rows.Close()
	type item struct {
		Path      string `json:"path"`
		Title     string `json:"title"`
		Summary   string `json:"summary"`
		UpdatedAt string `json:"updatedAt"`
	}
	out := []item{}
	for rows.Next() {
		var v item
		if err := rows.Scan(&v.Path, &v.Title, &v.Summary, &v.UpdatedAt); err != nil {
			jsonError(w, 500, err)
			return
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, out)
}

func parseSearchFilters(raw string) (query, tag, docType, folder string) {
	var terms []string
	var b strings.Builder
	quoted := false
	parts := []string{}
	flush := func() {
		if b.Len() > 0 {
			parts = append(parts, b.String())
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
	for _, part := range parts {
		switch {
		case strings.HasPrefix(part, "tag:"):
			tag = strings.Trim(strings.TrimPrefix(part, "tag:"), `"`)
		case strings.HasPrefix(part, "type:"):
			docType = strings.ToLower(strings.Trim(strings.TrimPrefix(part, "type:"), `"`))
		case strings.HasPrefix(part, "folder:"):
			folder = strings.Trim(strings.TrimPrefix(part, "folder:"), `"`)
		default:
			terms = append(terms, part)
		}
	}
	return strings.Join(terms, " "), tag, docType, folder
}

func (a *App) handleSemanticStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	writeJSON(w, a.embeddings.Status(ctx))
}

func (a *App) handleSemanticSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, []semantic.SemanticResult{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	results, err := a.embeddings.Search(ctx, q, 20)
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, map[string]any{"mode": "semantic", "model": a.embeddings.Model(), "results": results})
}

func (a *App) handleHybridSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, []map[string]any{})
		return
	}
	lexical, lexErr := a.db.Search(r.Context(), q)
	semantic, semErr := a.embeddings.Search(r.Context(), q, 50)
	type ranked struct {
		Path     string  `json:"path"`
		Title    string  `json:"title"`
		Snippet  string  `json:"snippet"`
		Score    float64 `json:"score"`
		Semantic float64 `json:"semanticSimilarity"`
	}
	byPath := map[string]*ranked{}
	for i, v := range lexical {
		x := byPath[v.Path]
		if x == nil {
			x = &ranked{Path: v.Path, Title: v.Title, Snippet: v.Snippet}
			byPath[v.Path] = x
		}
		x.Score += 1.0 / float64(60+i+1)
	}
	for i, v := range semantic {
		x := byPath[v.Path]
		if x == nil {
			x = &ranked{Path: v.Path, Title: v.Title, Snippet: v.Summary}
			byPath[v.Path] = x
		}
		x.Score += 1.0 / float64(60+i+1)
		x.Semantic = v.Similarity
	}
	out := make([]ranked, 0, len(byPath))
	for _, v := range byPath {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > 30 {
		out = out[:30]
	}
	writeJSON(w, map[string]any{"mode": "hybrid", "results": out, "lexicalAvailable": lexErr == nil, "semanticAvailable": semErr == nil, "semanticNote": errorString(semErr)})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
