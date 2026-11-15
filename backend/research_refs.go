package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"sefirot/internal/bibliography"
	"sefirot/internal/database"
)

const (
	maxReferenceImportBytes = 5 << 20
	maxSourceRecordBytes    = 256 << 10
)

type duplicateReference struct {
	Title      string `json:"title"`
	DOI        string `json:"doi"`
	ExistingID int64  `json:"existingId"`
}

type referenceImportReport struct {
	Format     string                   `json:"format"`
	Imported   []database.ResearchPaper `json:"imported"`
	Duplicates []duplicateReference     `json:"duplicates"`
	Errors     []string                 `json:"errors"`
}

func paperFromReference(ref bibliography.Reference, origin string) database.ResearchPaper {
	raw := ref.Raw
	if len(raw) > maxSourceRecordBytes {
		raw = ""
	}
	return database.ResearchPaper{
		Title: ref.Title, Authors: ref.Authors, Abstract: ref.Abstract, DOI: ref.DOI, Publication: ref.Publication,
		PublishedAt: ref.Year, URL: ref.URL, Keywords: ref.Keywords,
		MetadataOrigin: origin, EntryType: ref.EntryType, CiteKey: ref.Key, SourceRecord: raw,
	}
}

func (a *App) importReferences(w http.ResponseWriter, r *http.Request, entries []bibliography.Entry, format string, warnings []error) {
	report := referenceImportReport{Format: format, Imported: []database.ResearchPaper{}, Duplicates: []duplicateReference{}, Errors: []string{}}
	for _, e := range warnings {
		report.Errors = append(report.Errors, e.Error())
	}
	if len(entries) == 0 && len(warnings) == 0 {
		jsonError(w, 400, errors.New("no references found in the file"))
		return
	}
	for _, entry := range entries {
		ref := bibliography.FromEntry(entry)
		label := firstNonEmptyString(entry.Key, ref.Title)
		if strings.TrimSpace(ref.Title) == "" {
			report.Errors = append(report.Errors, label+": entry has no title and was skipped")
			continue
		}
		if existing, found, err := a.db.FindPaperByDOI(r.Context(), ref.DOI); err != nil {
			jsonError(w, 500, err)
			return
		} else if found {
			report.Duplicates = append(report.Duplicates, duplicateReference{Title: ref.Title, DOI: ref.DOI, ExistingID: existing.ID})
			continue
		}
		saved, err := a.db.SaveResearchPaper(r.Context(), paperFromReference(ref, format))
		if err != nil {
			report.Errors = append(report.Errors, label+": "+err.Error())
			continue
		}
		if err := a.persistResearch(r.Context(), saved.ID); err != nil {
			jsonError(w, 500, err)
			return
		}
		report.Imported = append(report.Imported, saved)
	}
	writeJSON(w, report)
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (a *App) handleResearchImport(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxReferenceImportBytes+1))
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if len(body) > maxReferenceImportBytes {
		jsonError(w, 413, errors.New("reference file exceeds 5 MiB"))
		return
	}
	text := strings.TrimPrefix(string(body), "\ufeff")
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		if strings.Contains(text, "TY  -") && !strings.Contains(text, "@") {
			format = "ris"
		} else {
			format = "bibtex"
		}
	}
	switch format {
	case "bibtex":
		entries, errs := bibliography.ParseBibTeX(text)
		a.importReferences(w, r, entries, "bibtex", errs)
	case "ris":
		entries, errs := bibliography.ParseRIS(text)
		a.importReferences(w, r, entries, "ris", errs)
	default:
		jsonError(w, 400, errors.New("format must be bibtex or ris"))
	}
}

func (a *App) selectedReferences(r *http.Request) ([]bibliography.Reference, error) {
	papers, err := a.db.ResearchPapers(r.Context())
	if err != nil {
		return nil, err
	}
	want := map[int64]bool{}
	for _, part := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if part = strings.TrimSpace(part); part != "" {
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil || id < 1 {
				return nil, fmt.Errorf("invalid id %q", part)
			}
			want[id] = true
		}
	}
	refs := []bibliography.Reference{}
	for i := len(papers) - 1; i >= 0; i-- {
		p := papers[i]
		if len(want) > 0 && !want[p.ID] {
			continue
		}
		base := strings.TrimSuffix(p.MetadataOrigin, "-edited")
		refs = append(refs, bibliography.Reference{
			Key: p.CiteKey, EntryType: p.EntryType, Title: p.Title, Authors: p.Authors, Publication: p.Publication,
			Year: p.PublishedAt, DOI: p.DOI, URL: p.URL, Abstract: p.Abstract, Keywords: p.Keywords,
			Raw: p.SourceRecord, Format: base, Edited: strings.HasSuffix(p.MetadataOrigin, "-edited"),
		})
	}
	return refs, nil
}

func (a *App) handleResearchExport(w http.ResponseWriter, r *http.Request) {
	refs, err := a.selectedReferences(r)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	var body, name string
	switch strings.ToLower(r.URL.Query().Get("format")) {
	case "", "bibtex":
		body, name = bibliography.FormatBibTeX(refs), "sefirot-references.bib"
	case "ris":
		body, name = bibliography.FormatRIS(refs), "sefirot-references.ris"
	default:
		jsonError(w, 400, errors.New("format must be bibtex or ris"))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = io.WriteString(w, body)
}

func (a *App) handleResearchBibliography(w http.ResponseWriter, r *http.Request) {
	refs, err := a.selectedReferences(r)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	style := strings.ToLower(r.URL.Query().Get("style"))
	if style == "" {
		style = "apa"
	}
	out, err := bibliography.FormatBibliography(refs, style)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, out)
}

// handleResearchLookupDOI contacts Crossref, an external service, and therefore only
// runs after the user has switched DOI lookup on in settings.
func (a *App) handleResearchLookupDOI(w http.ResponseWriter, r *http.Request) {
	s, err := a.settings.load()
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	if !s.DOILookup {
		jsonError(w, 403, errors.New("DOI lookup is turned off. Enable it in the research settings; it sends the DOI to api.crossref.org."))
		return
	}
	var req struct {
		DOI string `json:"doi"`
	}
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, 400, err)
		return
	}
	doi := bibliography.NormalizeDOI(req.DOI)
	if !bibliography.ValidDOI(doi) {
		jsonError(w, 400, errors.New("not a valid DOI"))
		return
	}
	if existing, found, err := a.db.FindPaperByDOI(r.Context(), doi); err != nil {
		jsonError(w, 500, err)
		return
	} else if found {
		writeJSON(w, map[string]any{"duplicate": true, "paper": existing})
		return
	}
	entry, err := a.crossref.Lookup(r.Context(), doi)
	if err != nil {
		jsonError(w, 502, err)
		return
	}
	saved, err := a.db.SaveResearchPaper(r.Context(), paperFromReference(bibliography.FromEntry(entry), "crossref"))
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if err := a.persistResearch(r.Context(), saved.ID); err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, map[string]any{"duplicate": false, "paper": saved})
}
