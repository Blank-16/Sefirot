package main

import (
	"net/http"
	"strconv"
)

func (a *App) handleAgentRuns(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	runs, err := a.db.AgentRuns(r.Context(), limit)
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, runs)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	var notes, links int
	if err := a.db.Reader.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM notes`).Scan(&notes); err != nil {
		jsonError(w, 500, err)
		return
	}
	if err := a.db.Reader.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM links`).Scan(&links); err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, map[string]any{"status": "ok", "notes": notes, "links": links, "database": "sqlite-fts5", "index": "filesystem-reconciled"})
}

func (a *App) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if !tokenEqual(r.Header.Get("X-Vault-Token"), a.terminalToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	a.shutdownOnce.Do(func() { close(a.shutdown) })
	writeJSON(w, map[string]bool{"shuttingDown": true})
}
