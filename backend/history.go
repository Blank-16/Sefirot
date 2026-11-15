package main

import (
	"fmt"
	"net/http"
	"path"
	"sefirot/internal/indexing"
	"sefirot/internal/vaultfs"
	"strings"
)

func (a *App) handleRevisions(w http.ResponseWriter, r *http.Request) {
	rel, err := indexing.SafeRelativePath(r.URL.Query().Get("path"))
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if !strings.EqualFold(path.Ext(rel), ".html") {
		jsonError(w, 400, fmt.Errorf("revision history is available for HTML notes only"))
		return
	}
	items, err := a.db.Revisions(r.Context(), rel)
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, items)
}

func (a *App) handleRestoreRevision(w http.ResponseWriter, r *http.Request) {
	var req restoreRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, 400, err)
		return
	}
	rel, err := indexing.SafeRelativePath(req.Path)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if !strings.EqualFold(path.Ext(rel), ".html") {
		jsonError(w, 400, fmt.Errorf("only HTML notes can be restored"))
		return
	}
	if req.RevisionID <= 0 {
		jsonError(w, 400, fmt.Errorf("revisionId must be positive"))
		return
	}
	content, err := a.db.RevisionContent(r.Context(), req.RevisionID, rel)
	if err != nil {
		jsonError(w, 404, err)
		return
	}
	if err = indexing.IndexFile(a.root, a.db, rel); err != nil {
		jsonError(w, 500, err)
		return
	}
	if err = a.db.SnapshotRevision(r.Context(), rel); err != nil {
		jsonError(w, 500, err)
		return
	}
	if err = vaultfs.WriteAtomic(a.root, rel, []byte(content)); err != nil {
		jsonError(w, 500, err)
		return
	}
	if err = indexing.IndexFile(a.root, a.db, rel); err != nil {
		jsonError(w, 500, err)
		return
	}
	payload, err := a.notePayload(r.Context(), rel)
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, payload)
}
