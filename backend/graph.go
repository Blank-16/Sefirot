package main

import (
	"errors"
	"net/http"
	"os"
	"sefirot/internal/indexing"
	"strings"
)

func (a *App) handleGraph(w http.ResponseWriter, r *http.Request) {
	nodes, edges, err := a.db.Graph(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, map[string]any{"nodes": nodes, "edges": edges})
}

func (a *App) handleAIContext(w http.ResponseWriter, r *http.Request) {
	rel, err := indexing.SafeRelativePath(r.URL.Query().Get("path"))
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	result, err := a.db.AIContext(r.Context(), rel)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || strings.Contains(strings.ToLower(err.Error()), "no rows") {
			http.NotFound(w, r)
		} else {
			jsonError(w, 500, err)
		}
		return
	}
	writeJSON(w, result)
}
