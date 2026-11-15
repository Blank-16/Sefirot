package bibliography

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleBib = `% a comment line
@string{jphys = "Journal of Physics"}
@comment{ignored @article{x, title={no}} }

@article{einstein1905,
  author = {Einstein, Albert and Podolsky, Boris},
  title = {On the {Electrodynamics} of Moving Bodies},
  journal = jphys # " A",
  year = 1905,
  month = jun,
  doi = {https://doi.org/10.1002/andp.19053221004},
  keywords = {relativity; physics, electrodynamics},
  abstract = "Quoted {abstract} with, a comma."
}

@book{broken,
  title = {Never closed
}

@inproceedings(knuth84, title={Literate Programming}, author={Knuth, Donald E.}, booktitle={Computer Journal}, year={1984})
`

func TestParseBibTeX(t *testing.T) {
	entries, errs := ParseBibTeX(sampleBib)
	if len(entries) != 2 {
		t.Fatalf("entries = %d (%v), want 2 (the malformed one skipped)", len(entries), entries)
	}
	if len(errs) == 0 {
		t.Fatal("malformed entry must be reported")
	}
	e := FromEntry(entries[0])
	if e.Key != "einstein1905" || e.Title != "On the Electrodynamics of Moving Bodies" {
		t.Fatalf("title/key: %+v", e)
	}
	if strings.Join(e.Authors, "|") != "Albert Einstein|Boris Podolsky" {
		t.Fatalf("authors: %v", e.Authors)
	}
	if e.Publication != "Journal of Physics A" || e.Year != "1905" || e.DOI != "10.1002/andp.19053221004" {
		t.Fatalf("fields: %+v", e)
	}
	if len(e.Keywords) != 3 || e.Abstract != "Quoted abstract with, a comma." {
		t.Fatalf("keywords/abstract: %v %q", e.Keywords, e.Abstract)
	}
	if !strings.Contains(entries[0].Raw, "On the {Electrodynamics}") || !strings.HasPrefix(entries[0].Raw, "@article{einstein1905") {
		t.Fatalf("raw record must be verbatim: %q", entries[0].Raw)
	}
	if k := FromEntry(entries[1]); k.Key != "knuth84" || k.Publication != "Computer Journal" {
		t.Fatalf("parenthesised entry: %+v", k)
	}
}

const sampleRIS = `TY  - JOUR
TI  - A very long title that
  continues here
AU  - Curie, Marie
AU  - Curie, Pierre
JO  - Annals
PY  - 1898///
DO  - 10.1000/xyz
KW  - radioactivity
KW  - polonium
ER  - 

TY  - BOOK
T1  - Second
A1  - Doe, Jane
ID  - doe1
ER  - 
`

func TestParseRIS(t *testing.T) {
	entries, errs := ParseRIS(sampleRIS)
	if len(entries) != 2 || len(errs) != 0 {
		t.Fatalf("entries=%d errs=%v", len(entries), errs)
	}
	r := FromEntry(entries[0])
	if r.Title != "A very long title that continues here" || r.Year != "1898" || len(r.Keywords) != 2 || r.Authors[0] != "Marie Curie" {
		t.Fatalf("ris mapping: %+v", r)
	}
	if !strings.Contains(entries[0].Raw, "TY  - JOUR") || !strings.Contains(entries[0].Raw, "ER  -") {
		t.Fatalf("raw: %q", entries[0].Raw)
	}
	if entries[1].Key != "doe1" {
		t.Fatalf("key from ID: %q", entries[1].Key)
	}
}

func TestExportIsVerbatimUnlessEdited(t *testing.T) {
	entries, _ := ParseBibTeX(sampleBib)
	ref := FromEntry(entries[0])
	out := FormatBibTeX([]Reference{ref})
	if !strings.Contains(out, "On the {Electrodynamics} of Moving Bodies") {
		t.Fatalf("unedited import must export verbatim: %s", out)
	}
	ref.Edited = true
	ref.Title = "Corrected title"
	out = FormatBibTeX([]Reference{ref})
	if !strings.Contains(out, "title = {Corrected title}") || !strings.Contains(out, "author = {Einstein, Albert and Podolsky, Boris}") {
		t.Fatalf("edited record must be regenerated from fields: %s", out)
	}
	again, errs := ParseBibTeX(out)
	if len(errs) != 0 || len(again) != 1 || FromEntry(again[0]).Title != "Corrected title" {
		t.Fatalf("generated BibTeX must parse back: %v %v", again, errs)
	}
}

func TestGeneratedKeysAreUnique(t *testing.T) {
	used := map[string]bool{}
	r := Reference{Authors: []string{"Ada Lovelace"}, Year: "1843"}
	a, b := GenerateKey(r, used), GenerateKey(r, used)
	if a != "lovelace1843" || b != "lovelace1843a" {
		t.Fatalf("keys: %s %s", a, b)
	}
}

func TestBibliographyStyles(t *testing.T) {
	refs := []Reference{
		{Title: "Zeta", Authors: []string{"Zed Zimmer"}, Year: "2001", Publication: "J. Z", DOI: "10.1/z"},
		{Title: "Alpha", Authors: []string{"Ann Adams", "Bob Brown"}, Year: "1999"},
	}
	apa, err := FormatBibliography(refs, "apa")
	if err != nil || !strings.HasPrefix(apa, "Adams, A., & Brown, B. (1999). Alpha.") || !strings.Contains(apa, "https://doi.org/10.1/z") {
		t.Fatalf("apa: %q %v", apa, err)
	}
	ieee, _ := FormatBibliography(refs, "ieee")
	if !strings.HasPrefix(ieee, "[1] Z. Zimmer, \"Zeta,\" J. Z, 2001.") {
		t.Fatalf("ieee keeps given order: %q", ieee)
	}
	if _, err := FormatBibliography(refs, "chicago-fancy"); err == nil {
		t.Fatal("unknown style must error")
	}
}

func TestNormalizeDOI(t *testing.T) {
	for in, want := range map[string]string{"doi:10.1/a": "10.1/a", "HTTPS://DOI.ORG/10.1/a": "10.1/a", " 10.1/a ": "10.1/a"} {
		if got := NormalizeDOI(in); got != want {
			t.Errorf("%q -> %q", in, got)
		}
	}
}

func TestCrossrefLookup(t *testing.T) {
	var gotPath, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotUA = r.URL.EscapedPath(), r.Header.Get("User-Agent")
		if strings.HasSuffix(r.URL.Path, "10.1234/missing") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"ok","message":{"DOI":"10.1234/abc","type":"journal-article","title":["Found It"],"author":[{"given":"Ada","family":"Lovelace"},{"name":"The Consortium"}],"container-title":["Annals"],"issued":{"date-parts":[[2021,3]]},"abstract":"<jats:p>Abstract <i>text</i>.</jats:p>","URL":"https://doi.org/10.1234/abc","subject":["Math"]}}`)
	}))
	defer srv.Close()
	c := Crossref{BaseURL: srv.URL}
	e, err := c.Lookup(t.Context(), "https://doi.org/10.1234/abc")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/works/10.1234/abc" || !strings.Contains(gotUA, "SEFIROT") {
		t.Fatalf("request: path=%q ua=%q", gotPath, gotUA)
	}
	r := FromEntry(e)
	if r.Title != "Found It" || len(r.Authors) != 2 || r.Authors[0] != "Ada Lovelace" || r.Year != "2021" || r.Abstract != "Abstract text ." {
		t.Fatalf("mapping: %+v", r)
	}
	if !strings.Contains(e.Raw, `"status":"ok"`) {
		t.Fatal("raw crossref JSON must be preserved")
	}
	if _, err := c.Lookup(t.Context(), "10.1234/missing"); err == nil || !strings.Contains(err.Error(), "no record") {
		t.Fatalf("404: %v", err)
	}
	if _, err := c.Lookup(t.Context(), "not a doi"); err == nil {
		t.Fatal("invalid DOI must be rejected before any request")
	}
}
