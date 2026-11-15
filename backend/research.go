package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"sefirot/internal/database"
	"sefirot/internal/vaultfs"
)

func researchID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid research paper id")
	}
	return id, nil
}
func (a *App) handleResearchList(w http.ResponseWriter, r *http.Request) {
	items, err := a.db.ResearchPapers(r.Context())
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}
func (a *App) handleResearchCreate(w http.ResponseWriter, r *http.Request) {
	var p database.ResearchPaper
	if err := decodeJSON(r, &p); err != nil {
		jsonError(w, 400, err)
		return
	}
	p.MetadataOrigin, p.EntryType, p.CiteKey, p.SourceRecord = "", "", "", ""
	p.SourceFile = "" // Source files can only be attached through the validated PDF upload endpoint.
	saved, err := a.db.SaveResearchPaper(r.Context(), p)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if err := a.persistResearch(r.Context(), saved.ID); err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, saved)
}
func (a *App) handleResearchGet(w http.ResponseWriter, r *http.Request) {
	id, err := researchID(r)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	p, err := a.db.ResearchPaper(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	citations, err := a.db.ResearchCitations(r.Context(), id)
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	annotations, err := a.db.ResearchAnnotations(r.Context(), id)
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, map[string]any{"paper": p, "citations": citations, "annotations": annotations})
}
func (a *App) handleResearchUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := researchID(r)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	var p database.ResearchPaper
	if err := decodeJSON(r, &p); err != nil {
		jsonError(w, 400, err)
		return
	}
	existing, err := a.db.ResearchPaper(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	p.ID = id
	p.SourceFile = existing.SourceFile // PDF attachment paths are server-managed.
	p.CreatedAt = existing.CreatedAt
	saved, err := a.db.SaveResearchPaper(r.Context(), p)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if err := a.persistResearch(r.Context(), saved.ID); err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, saved)
}
func (a *App) handleResearchDelete(w http.ResponseWriter, r *http.Request) {
	id, err := researchID(r)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	paper, err := a.db.ResearchPaper(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	if _, err := vaultfs.Trash(a.root, researchRecordPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		jsonError(w, 500, fmt.Errorf("could not move the vault record to trash: %w", err))
		return
	}
	if paper.SourceFile != "" {
		if _, err := vaultfs.Trash(a.root, paper.SourceFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("research pdf trash %s: %v", paper.SourceFile, err)
		}
	}
	if err := a.db.DeleteResearchPaper(r.Context(), id); err != nil {
		jsonError(w, 500, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *App) handleResearchCitations(w http.ResponseWriter, r *http.Request) {
	id, err := researchID(r)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	items, err := a.db.ResearchCitations(r.Context(), id)
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}
func (a *App) handleResearchAddCitation(w http.ResponseWriter, r *http.Request) {
	id, err := researchID(r)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	var c database.ResearchCitation
	if err := decodeJSON(r, &c); err != nil {
		jsonError(w, 400, err)
		return
	}
	c.PaperID = id
	c.Origin = "manual"
	saved, err := a.db.AddResearchCitation(r.Context(), c)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if err := a.persistResearch(r.Context(), id); err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, saved)
}
func (a *App) handleResearchAnnotations(w http.ResponseWriter, r *http.Request) {
	id, err := researchID(r)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	items, err := a.db.ResearchAnnotations(r.Context(), id)
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}
func (a *App) handleResearchAddAnnotation(w http.ResponseWriter, r *http.Request) {
	id, err := researchID(r)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	var item database.ResearchAnnotation
	if err := decodeJSON(r, &item); err != nil {
		jsonError(w, 400, err)
		return
	}
	item.Origin = "manual"
	if item.Page < 0 || strings.TrimSpace(item.Note) == "" {
		jsonError(w, 400, errors.New("page must be non-negative and note is required"))
		return
	}
	item.PaperID = id
	saved, err := a.db.AddResearchAnnotation(r.Context(), item)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if err := a.persistResearch(r.Context(), id); err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, saved)
}

// PDF files are preserved verbatim inside the vault. Parsing is delegated to an
// optional local GROBID service in a later ingestion layer; uploads never leave
// the machine as part of this operation.
func (a *App) handleResearchUploadPDF(w http.ResponseWriter, r *http.Request) {
	id, err := researchID(r)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	paper, err := a.db.ResearchPaper(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	const maxPDF = 100 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxPDF)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		jsonError(w, 400, fmt.Errorf("invalid PDF upload or file exceeds 100 MiB: %w", err))
		return
	}
	file, _, err := r.FormFile("pdf")
	if err != nil {
		jsonError(w, 400, errors.New("select a PDF file using the pdf field"))
		return
	}
	defer file.Close()
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	data, err := io.ReadAll(io.LimitReader(file, maxPDF+1))
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if len(data) > maxPDF {
		jsonError(w, 413, errors.New("PDF exceeds 100 MiB limit"))
		return
	}
	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		jsonError(w, 400, errors.New("uploaded file is not a PDF"))
		return
	}
	name := fmt.Sprintf("research/%d/%d-source.pdf", id, time.Now().UTC().UnixNano())
	if err := vaultfs.WriteAtomic(a.root, name, data); err != nil {
		jsonError(w, 500, err)
		return
	}
	oldSource := paper.SourceFile
	paper.SourceFile = path.Clean(name)
	saved, err := a.db.SaveResearchPaper(r.Context(), paper)
	if err != nil {
		_ = a.root.Remove(name)
		jsonError(w, 500, err)
		return
	}
	if oldSource != "" && oldSource != name {
		if _, err := vaultfs.Trash(a.root, oldSource); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("replaced pdf trash %s: %v", oldSource, err)
		}
	}
	if err := a.persistResearch(r.Context(), id); err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, saved)
}

func (a *App) handleResearchDeleteCitation(w http.ResponseWriter, r *http.Request) {
	a.deleteResearchChild(w, r, "cid", a.db.DeleteResearchCitation)
}

func (a *App) handleResearchDeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	a.deleteResearchChild(w, r, "aid", a.db.DeleteResearchAnnotation)
}

func (a *App) deleteResearchChild(w http.ResponseWriter, r *http.Request, param string, del func(context.Context, int64, int64) error) {
	id, err := researchID(r)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	childID, err := strconv.ParseInt(r.PathValue(param), 10, 64)
	if err != nil || childID < 1 {
		jsonError(w, 400, errors.New("invalid id"))
		return
	}
	if err := del(r.Context(), id, childID); errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		jsonError(w, 500, err)
		return
	}
	if err := a.persistResearch(r.Context(), id); err != nil {
		jsonError(w, 500, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
