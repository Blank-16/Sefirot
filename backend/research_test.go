package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"sefirot/internal/database"
)

func seedPaper(t *testing.T, a *testApp) int64 {
	t.Helper()
	rec, out := a.do(t, "POST", "/api/research", map[string]any{"title": "On Things", "authors": []string{"A. Author"}, "doi": "10.1/abc", "abstract": "x \\( \\alpha \\)"})
	expect(t, rec, 200)
	id := int64(out["id"].(float64))
	p := "/api/research/" + itoa(id)
	rec, _ = a.do(t, "POST", p+"/citations", map[string]any{"citationText": "Ref One", "locator": "p. 3"})
	expect(t, rec, 200)
	rec, _ = a.do(t, "POST", p+"/annotations", map[string]any{"page": 2, "quote": "q", "note": "n"})
	expect(t, rec, 200)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("pdf", "x.pdf")
	fw.Write([]byte("%PDF-1.4 test"))
	mw.Close()
	req := httptest.NewRequest("POST", "http://127.0.0.1:43999"+p+"/pdf", &body)
	req.Host = "127.0.0.1:43999"
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	a.h.ServeHTTP(w, req)
	expect(t, w, 200)
	return id
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

func TestResearchSurvivesIndexLoss(t *testing.T) {
	a := newTestApp(t)
	id := seedPaper(t, a)
	if _, err := os.Stat(filepath.Join(a.vaultDir, "research", itoa(id), "record.json")); err != nil {
		t.Fatalf("vault record missing: %v", err)
	}
	fresh, err := database.OpenDatabase(filepath.Join(a.dir, "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	b := &App{root: a.root, db: fresh}
	n, err := b.rebuildResearch(t.Context())
	if err != nil || n != 1 {
		t.Fatalf("rebuild = %d, %v", n, err)
	}
	rec, err := fresh.ResearchRecord(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Paper.Title != "On Things" || rec.Paper.DOI != "10.1/abc" || len(rec.Citations) != 1 || len(rec.Annotations) != 1 || rec.Paper.SourceFile == "" {
		t.Fatalf("restored record incomplete: %+v", rec)
	}
	n, _ = b.rebuildResearch(t.Context())
	if n != 0 {
		t.Fatalf("second rebuild must be a no-op, restored %d", n)
	}
}

func TestResearchExportImportRoundTrip(t *testing.T) {
	src := newTestApp(t)
	id := seedPaper(t, src)
	rec, _ := src.do(t, "GET", "/api/export", nil)
	expect(t, rec, 200)
	dst := newTestApp(t)
	r2, out := dst.do(t, "POST", "/api/import", rec.Body.Bytes())
	expect(t, r2, 200)
	if len(out["indexErrors"].([]any)) != 0 {
		t.Fatalf("index errors: %v", out["indexErrors"])
	}
	r3, got := dst.do(t, "GET", "/api/research/"+itoa(id), nil)
	expect(t, r3, 200)
	if len(got["citations"].([]any)) != 1 || len(got["annotations"].([]any)) != 1 {
		t.Fatalf("children lost in round trip: %v", got)
	}
}

func TestResearchDeleteIsRecoverable(t *testing.T) {
	a := newTestApp(t)
	id := seedPaper(t, a)
	p := "/api/research/" + itoa(id)
	rec, _ := a.do(t, "DELETE", p, nil)
	expect(t, rec, http.StatusNoContent)
	rec, _ = a.do(t, "GET", p, nil)
	expect(t, rec, 404)
	if n, _ := a.rebuildResearch(t.Context()); n != 0 {
		t.Fatal("deleted paper must not be resurrected by rebuild")
	}
	_, list := a.do(t, "GET", "/api/trash", nil)
	var recordTrash string
	for _, it := range list["items"].([]any) {
		m := it.(map[string]any)
		if m["path"] == "research/"+itoa(id)+"/record.json" {
			recordTrash = m["id"].(string)
		}
	}
	if recordTrash == "" {
		t.Fatalf("record not in trash: %v", list)
	}
	rec, _ = a.do(t, "POST", "/api/trash/restore", map[string]any{"id": recordTrash})
	expect(t, rec, 200)
	rec, _ = a.do(t, "GET", p, nil)
	expect(t, rec, 200)
}

func TestResearchChildDeletion(t *testing.T) {
	a := newTestApp(t)
	id := seedPaper(t, a)
	p := "/api/research/" + itoa(id)
	_, got := a.do(t, "GET", p, nil)
	cid := int64(got["citations"].([]any)[0].(map[string]any)["id"].(float64))
	aid := int64(got["annotations"].([]any)[0].(map[string]any)["id"].(float64))
	rec, _ := a.do(t, "DELETE", p+"/citations/"+itoa(cid), nil)
	expect(t, rec, http.StatusNoContent)
	rec, _ = a.do(t, "DELETE", p+"/annotations/"+itoa(aid), nil)
	expect(t, rec, http.StatusNoContent)
	rec, _ = a.do(t, "DELETE", p+"/annotations/"+itoa(aid), nil)
	expect(t, rec, 404)
	r, err := a.db.ResearchRecord(t.Context(), id)
	if err != nil || len(r.Citations) != 0 || len(r.Annotations) != 0 {
		t.Fatalf("children not removed: %+v %v", r, err)
	}
	raw, _ := os.ReadFile(filepath.Join(a.vaultDir, "research", itoa(id), "record.json"))
	if bytes.Contains(raw, []byte("Ref One")) {
		t.Fatal("vault record still holds deleted citation")
	}
}

const testBib = `@article{ein1905, author={Einstein, Albert}, title={On the {Electrodynamics} of Moving Bodies}, journal={Annalen}, year={1905}, doi={10.1002/andp.19053221004}}
@book{nodoi, author={Doe, Jane}, title={Book Without DOI}, year={2001}}
@misc{notitle, author={Nobody}}
@article{broken, title={Unclosed`

func TestReferenceImportExportRoundTrip(t *testing.T) {
	a := newTestApp(t)
	rec, out := a.do(t, "POST", "/api/research/import", []byte(testBib))
	expect(t, rec, 200)
	if len(out["imported"].([]any)) != 2 {
		t.Fatalf("imported: %v", out)
	}
	if len(out["errors"].([]any)) < 2 {
		t.Fatalf("a missing title and a malformed entry must both be reported: %v", out["errors"])
	}
	first := out["imported"].([]any)[0].(map[string]any)
	if first["metadataOrigin"] != "bibtex" || first["citeKey"] != "ein1905" || first["title"] != "On the Electrodynamics of Moving Bodies" {
		t.Fatalf("imported paper: %v", first)
	}
	if !strings.Contains(first["sourceRecord"].(string), "{Electrodynamics}") {
		t.Fatal("source record must be stored verbatim")
	}
	rec, out = a.do(t, "POST", "/api/research/import", []byte(testBib))
	expect(t, rec, 200)
	if len(out["imported"].([]any)) != 1 || len(out["duplicates"].([]any)) != 1 {
		t.Fatalf("re-import must skip the DOI duplicate: %v", out)
	}
	rec, _ = a.do(t, "GET", "/api/research/export?format=bibtex", nil)
	expect(t, rec, 200)
	if !strings.Contains(rec.Body.String(), "On the {Electrodynamics} of Moving Bodies") || !strings.Contains(rec.Header().Get("Content-Disposition"), "sefirot-references.bib") {
		t.Fatalf("export: %q %v", rec.Body.String(), rec.Header())
	}
	id := int64(first["id"].(float64))
	p := "/api/research/" + itoa(id)
	_, detail := a.do(t, "GET", p, nil)
	paper := detail["paper"].(map[string]any)
	paper["title"] = "A Corrected Title"
	rec, _ = a.do(t, "PUT", p, paper)
	expect(t, rec, 200)
	_, detail = a.do(t, "GET", p, nil)
	if detail["paper"].(map[string]any)["metadataOrigin"] != "bibtex-edited" {
		t.Fatalf("edit must be flagged: %v", detail["paper"])
	}
	rec, _ = a.do(t, "GET", "/api/research/export?format=bibtex&ids="+itoa(id), nil)
	if !strings.Contains(rec.Body.String(), "A Corrected Title") || strings.Contains(rec.Body.String(), "Moving Bodies") {
		t.Fatalf("edited record must export current fields: %q", rec.Body.String())
	}
	rec, _ = a.do(t, "GET", "/api/research/bibliography?style=apa", nil)
	expect(t, rec, 200)
	if !strings.Contains(rec.Body.String(), "Doe, J. (2001). Book Without DOI.") {
		t.Fatalf("bibliography: %q", rec.Body.String())
	}
	rec, _ = a.do(t, "GET", "/api/research/bibliography?style=nope", nil)
	expect(t, rec, 400)
}

func TestRISImportAndClientCannotForgeProvenance(t *testing.T) {
	a := newTestApp(t)
	rec, out := a.do(t, "POST", "/api/research/import", []byte("TY  - JOUR\nTI  - RIS Paper\nAU  - Curie, Marie\nPY  - 1898\nDO  - 10.1000/ris1\nER  - \n"))
	expect(t, rec, 200)
	if len(out["imported"].([]any)) != 1 || out["format"] != "ris" {
		t.Fatalf("ris import: %v", out)
	}
	rec, created := a.do(t, "POST", "/api/research", map[string]any{"title": "Mine", "metadataOrigin": "crossref", "sourceRecord": "forged", "citeKey": "forged"})
	expect(t, rec, 200)
	if created["metadataOrigin"] != "manual" || created["sourceRecord"] != nil {
		t.Fatalf("client-supplied provenance must be ignored: %v", created)
	}
	id := itoa(int64(created["id"].(float64)))
	rec, ann := a.do(t, "POST", "/api/research/"+id+"/annotations", map[string]any{"note": "n", "origin": "ai"})
	expect(t, rec, 200)
	if ann["origin"] != "manual" {
		t.Fatalf("annotation origin must be set by the server: %v", ann)
	}
	rec, _ = a.do(t, "POST", "/api/research/import?format=xml", []byte("x"))
	expect(t, rec, 400)
	rec, _ = a.do(t, "POST", "/api/research/import", []byte("no references here"))
	expect(t, rec, 400)
}

func TestDOILookupIsOptInAndRecordsProvenance(t *testing.T) {
	a := newTestApp(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":{"DOI":"10.1234/found","title":["Looked Up"],"author":[{"given":"A","family":"B"}],"issued":{"date-parts":[[2020]]}}}`))
	}))
	defer srv.Close()
	a.crossref.BaseURL = srv.URL
	rec, _ := a.do(t, "POST", "/api/research/lookup-doi", map[string]any{"doi": "10.1234/found"})
	expect(t, rec, 403)
	if calls != 0 {
		t.Fatal("no request may leave the machine while lookup is disabled")
	}
	rec, _ = a.do(t, "PUT", "/api/settings", map[string]any{"theme": "dark", "doiLookup": true})
	expect(t, rec, 200)
	rec, out := a.do(t, "POST", "/api/research/lookup-doi", map[string]any{"doi": "https://doi.org/10.1234/found"})
	expect(t, rec, 200)
	paper := out["paper"].(map[string]any)
	if paper["metadataOrigin"] != "crossref" || paper["title"] != "Looked Up" || !strings.Contains(paper["sourceRecord"].(string), `"Looked Up"`) {
		t.Fatalf("lookup result: %v", paper)
	}
	rec, out = a.do(t, "POST", "/api/research/lookup-doi", map[string]any{"doi": "10.1234/found"})
	expect(t, rec, 200)
	if out["duplicate"] != true || calls != 1 {
		t.Fatalf("existing DOI must not be fetched again: %v calls=%d", out, calls)
	}
	rec, _ = a.do(t, "POST", "/api/research/lookup-doi", map[string]any{"doi": "nonsense"})
	expect(t, rec, 400)
}
