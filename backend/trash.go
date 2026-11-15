package main

import (
	"errors"
	"net/http"
	"os"
	"path"

	"sefirot/internal/indexing"
	"sefirot/internal/vaultfs"
)

func (a *App) handleTrashList(w http.ResponseWriter, r *http.Request) {
	items, err := vaultfs.ListTrash(a.root)
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "retentionDays": int(vaultfs.TrashRetention.Hours() / 24)})
}

func (a *App) handleTrashRestore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   string `json:"id"`
		Path string `json:"path,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, 400, err)
		return
	}
	dest, err := vaultfs.RestoreTrash(a.root, req.ID, req.Path)
	switch {
	case errors.Is(err, vaultfs.ErrExists):
		jsonError(w, 409, err)
		return
	case errors.Is(err, os.ErrNotExist):
		http.NotFound(w, r)
		return
	case err != nil:
		jsonError(w, 400, err)
		return
	}
	if path.Base(dest) == researchRecordName && path.Dir(path.Dir(dest)) == "research" {
		if _, err := a.rebuildResearch(r.Context()); err != nil {
			jsonError(w, 500, err)
			return
		}
	}
	if err := indexing.IndexFile(a.root, a.db, dest); err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, map[string]string{"path": dest})
}

func (a *App) handleTrashPurge(w http.ResponseWriter, r *http.Request) {
	if err := vaultfs.PurgeTrash(a.root, r.URL.Query().Get("id")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		jsonError(w, 400, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
