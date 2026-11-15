package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"sefirot/internal/indexing"
	"sefirot/internal/vaultfs"
	"strings"
)

func (a *App) handleTree(w http.ResponseWriter, r *http.Request) {
	titles, err := a.db.Titles(r.Context())
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	rootNode := TreeNode{Name: "Vault", Path: "", Type: "folder", Children: []TreeNode{}}
	var add func(*TreeNode, string) error
	add = func(parent *TreeNode, rel string) error {
		entries, err := indexing.ReadDir(a.root, rel)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Type()&os.ModeSymlink != 0 {
				continue
			}
			if indexing.IsReserved(e.Name()) && (rel == "." || rel == "") {
				continue
			}
			childRel := e.Name()
			if rel != "." && rel != "" {
				childRel = path.Join(rel, e.Name())
			}
			if e.IsDir() {
				node := TreeNode{Name: e.Name(), Path: childRel, Type: "folder", Children: []TreeNode{}}
				if err := add(&node, childRel); err != nil {
					return err
				}
				parent.Children = append(parent.Children, node)
			} else if strings.EqualFold(path.Ext(e.Name()), ".html") {
				title := titles[childRel]
				if title == "" {
					title = strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
				}
				parent.Children = append(parent.Children, TreeNode{Name: e.Name(), Path: childRel, Type: "note", Title: title})
			}
		}
		return nil
	}
	if err := add(&rootNode, "."); err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, rootNode)
}

func (a *App) handleGetNote(w http.ResponseWriter, r *http.Request) {
	rel, err := indexing.SafeRelativePath(r.URL.Query().Get("path"))
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if !strings.EqualFold(path.Ext(rel), ".html") {
		jsonError(w, 400, fmt.Errorf("notes must use .html extension"))
		return
	}
	if err := indexing.IndexFile(a.root, a.db, rel); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
		} else {
			jsonError(w, 500, err)
		}
		return
	}
	payload, err := a.notePayload(r.Context(), rel)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
		} else {
			jsonError(w, 500, err)
		}
		return
	}
	writeJSON(w, payload)
}

func (a *App) notePayload(ctx context.Context, rel string) (NoteResponse, error) {
	raw, err := indexing.ReadRootFile(a.root, rel)
	if err != nil {
		return NoteResponse{}, err
	}
	content, isMD := indexing.ExtractMarkdownSource(raw)
	rendered := string(indexing.StripSourceComment(raw))
	title, _, _, err := indexing.ExtractHTML(indexing.StripSourceComment(raw))
	if err != nil {
		return NoteResponse{}, err
	}
	backlinks, err := a.db.Backlinks(ctx, rel)
	if err != nil {
		return NoteResponse{}, err
	}
	details, err := a.db.Details(ctx, rel)
	if err != nil {
		return NoteResponse{}, err
	}
	return NoteResponse{Path: rel, Title: title, Content: content, RenderedHTML: rendered, IsMarkdown: isMD, Backlinks: backlinks, Summary: details.Summary, SummaryShort: details.SummaryShort, SummaryDetail: details.SummaryDetail, Headings: details.Headings, Description: details.Description, DocType: details.DocType, Tags: details.Tags, ContentHash: details.ContentHash, UpdatedAt: details.UpdatedAt}, nil
}

func (a *App) handleSaveNote(w http.ResponseWriter, r *http.Request) {
	var req noteRequest
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
		jsonError(w, 400, fmt.Errorf("note path must end in .html"))
		return
	}
	var output string
	if req.IsMarkdown {
		output, _, err = indexing.RenderMarkdown(req.Content)
	} else {
		output = req.Content
		if strings.TrimSpace(output) == "" {
			output = "<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>Untitled</title></head><body></body></html>"
		}
	}
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if req.ExpectedHash != "" && !req.Force && !req.CreateOnly {
		current, readErr := indexing.ReadRootFile(a.root, rel)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				jsonError(w, 409, fmt.Errorf("note was removed outside SEFIROT"))
				return
			}
			jsonError(w, 500, readErr)
			return
		}
		currentHash := sha256.Sum256(current)
		if hex.EncodeToString(currentHash[:]) != req.ExpectedHash {
			jsonError(w, 409, fmt.Errorf("note changed outside SEFIROT; reload it or explicitly overwrite the external changes"))
			return
		}
	}
	if req.Force && req.ExpectedHash != "" {
		if _, readErr := indexing.ReadRootFile(a.root, rel); readErr == nil {
			if indexErr := indexing.IndexFile(a.root, a.db, rel); indexErr != nil {
				jsonError(w, 500, indexErr)
				return
			}
		} else if !errors.Is(readErr, os.ErrNotExist) {
			jsonError(w, 500, readErr)
			return
		}
	}
	if err = indexing.EnsureRootDirs(a.root, rel); err != nil {
		jsonError(w, 400, err)
		return
	}
	if req.CreateOnly {
		created, createErr := vaultfs.WriteExclusive(a.root, rel, []byte(output))
		if createErr != nil {
			jsonError(w, 500, createErr)
			return
		}
		if !created {
			jsonError(w, 409, fmt.Errorf("a note already exists at %s", rel))
			return
		}
	} else if err = vaultfs.WriteAtomic(a.root, rel, []byte(output)); err != nil {
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

func (a *App) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, 400, err)
		return
	}
	rel, err := indexing.SafeRelativePath(req.Path)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	current := ""
	for _, part := range strings.Split(rel, "/") {
		current = path.Join(current, part)
		if err := a.root.Mkdir(current, 0755); err != nil && !errors.Is(err, os.ErrExist) {
			jsonError(w, 400, err)
			return
		}
	}
	writeJSON(w, map[string]string{"path": rel})
}

func (a *App) handleRename(w http.ResponseWriter, r *http.Request) {
	var req renameRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, 400, err)
		return
	}
	oldPath, err := indexing.SafeRelativePath(req.OldPath)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	newPath, err := indexing.SafeRelativePath(req.NewPath)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if !strings.EqualFold(path.Ext(oldPath), ".html") || !strings.EqualFold(path.Ext(newPath), ".html") {
		jsonError(w, 400, fmt.Errorf("both paths must end in .html"))
		return
	}
	if err = indexing.EnsureRootDirs(a.root, newPath); err != nil {
		jsonError(w, 400, err)
		return
	}
	if err = a.db.Rename(r.Context(), oldPath, newPath); err != nil {
		jsonError(w, 409, err)
		return
	}
	if err = vaultfs.Move(a.root, oldPath, newPath); err != nil {
		_ = a.db.Rename(r.Context(), newPath, oldPath)
		status := 400
		if errors.Is(err, vaultfs.ErrExists) {
			status = 409
		}
		jsonError(w, status, err)
		return
	}
	if err = indexing.IndexFile(a.root, a.db, newPath); err != nil {
		jsonError(w, 500, fmt.Errorf("file renamed but index update failed: %w", err))
		return
	}
	writeJSON(w, map[string]string{"path": newPath})
}

func (a *App) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	rel, err := indexing.SafeRelativePath(r.URL.Query().Get("path"))
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if !strings.EqualFold(path.Ext(rel), ".html") {
		jsonError(w, 400, fmt.Errorf("only .html notes can be deleted"))
		return
	}
	entry, err := vaultfs.Trash(a.root, rel)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
		} else {
			jsonError(w, 400, err)
		}
		return
	}
	if err = a.db.Delete(r.Context(), rel); err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "trashId": entry.ID})
}
