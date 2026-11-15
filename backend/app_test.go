package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sefirot/internal/database"
	"sefirot/internal/indexing"
	"sefirot/internal/semantic"
)

const testPort = 43999

type testApp struct {
	*App
	dir string
	h   http.Handler
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	dir := t.TempDir()
	vault := filepath.Join(dir, "vault")
	if err := os.MkdirAll(vault, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(vault)
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.OpenDatabase(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); root.Close() })
	a := &App{root: root, vaultDir: vault, port: testPort, baseURL: "http://127.0.0.1:43999", db: db, settings: &settingsStore{path: filepath.Join(dir, "settings.json")}, terminalToken: "t", terminals: map[string]*terminalSession{}, shutdown: make(chan struct{})}
	a.embeddings = semantic.NewEmbeddingService(db)
	return &testApp{App: a, dir: dir, h: a.routes()}
}

func (a *testApp) do(t *testing.T, method, url string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var reader io.Reader
	switch v := body.(type) {
	case nil:
	case []byte:
		reader = bytes.NewReader(v)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, "http://127.0.0.1:43999"+url, reader)
	req.Host = "127.0.0.1:43999"
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	var out map[string]any
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec, out
}

func (a *testApp) save(t *testing.T, p, content string, extra map[string]any) (*httptest.ResponseRecorder, map[string]any) {
	body := map[string]any{"path": p, "content": content, "isMarkdown": true}
	for k, v := range extra {
		body[k] = v
	}
	return a.do(t, "POST", "/api/notes", body)
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, status, rec.Body.String())
	}
}

func TestSaveLoadAndConflict(t *testing.T) {
	a := newTestApp(t)
	rec, note := a.save(t, "a.html", "# Alpha\n\nbody", map[string]any{"createOnly": true})
	expect(t, rec, 200)
	hash := note["contentHash"].(string)
	rec, _ = a.save(t, "a.html", "# Alpha\n\nbody", map[string]any{"createOnly": true})
	expect(t, rec, 409)
	if err := os.WriteFile(filepath.Join(a.vaultDir, "a.html"), []byte("<html><body>external</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, _ = a.save(t, "a.html", "# mine", map[string]any{"expectedHash": hash})
	expect(t, rec, 409)
	rec, note = a.save(t, "a.html", "# mine", map[string]any{"expectedHash": hash, "force": true})
	expect(t, rec, 200)
	if note["content"] != "# mine" {
		t.Fatalf("content = %v", note["content"])
	}
	rec, _ = a.do(t, "GET", "/api/revisions-missing", nil)
	expect(t, rec, 404)
}

func TestReservedPathsRejected(t *testing.T) {
	a := newTestApp(t)
	for _, p := range []string{".sefirot/x.html", ".SEFIROT/x.html", ".sefirot/trash/y/content.html"} {
		rec, _ := a.save(t, p, "x", nil)
		expect(t, rec, 400)
	}
	rec, _ := a.do(t, "GET", "/vault-assets/.sefirot/trash/x", nil)
	expect(t, rec, 400)
}

func TestSaveLeavesNoStagingFilesAndIsAtomic(t *testing.T) {
	a := newTestApp(t)
	rec, _ := a.save(t, "n.html", "# first", nil)
	expect(t, rec, 200)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 40; i++ {
			a.save(t, "n.html", "# version "+strings.Repeat("x", 4000*i), nil)
		}
	}()
	for {
		select {
		case <-done:
			entries, _ := os.ReadDir(filepath.Join(a.vaultDir, ".sefirot", "tmp"))
			if len(entries) != 0 {
				t.Fatalf("staging files left behind: %v", entries)
			}
			return
		default:
			raw, err := os.ReadFile(filepath.Join(a.vaultDir, "n.html"))
			if err != nil || len(raw) == 0 {
				t.Fatalf("reader observed missing or truncated file: len=%d err=%v", len(raw), err)
			}
		}
	}
}

func TestDeleteGoesToTrashAndRestores(t *testing.T) {
	a := newTestApp(t)
	expect(t, firstOf(a.save(t, "Dir/b.html", "# Beta", nil)), 200)
	rec, out := a.do(t, "DELETE", "/api/notes?path=Dir/b.html", nil)
	expect(t, rec, 200)
	if out["trashId"] == nil {
		t.Fatal("expected trashId")
	}
	if _, err := os.Stat(filepath.Join(a.vaultDir, "Dir", "b.html")); !os.IsNotExist(err) {
		t.Fatalf("note should be gone from vault: %v", err)
	}
	rec, list := a.do(t, "GET", "/api/trash", nil)
	expect(t, rec, 200)
	items := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("trash items = %v", items)
	}
	rec, tree := a.do(t, "GET", "/api/tree", nil)
	expect(t, rec, 200)
	if strings.Contains(rec.Body.String(), ".sefirot") {
		t.Fatalf("tree leaks reserved directory: %v", tree)
	}
	rec, results := a.do(t, "GET", "/api/search?q=Beta", nil)
	expect(t, rec, 200)
	_ = results
	id := items[0].(map[string]any)["id"].(string)
	rec, _ = a.do(t, "POST", "/api/trash/restore", map[string]any{"id": id})
	expect(t, rec, 200)
	rec, note := a.do(t, "GET", "/api/notes?path=Dir/b.html", nil)
	expect(t, rec, 200)
	if note["content"] != "# Beta" {
		t.Fatalf("restored content = %v", note["content"])
	}
	rec, _ = a.do(t, "POST", "/api/trash/restore", map[string]any{"id": id})
	expect(t, rec, 404)
}

func TestRestoreRefusesToOverwrite(t *testing.T) {
	a := newTestApp(t)
	expect(t, firstOf(a.save(t, "c.html", "# one", nil)), 200)
	_, out := a.do(t, "DELETE", "/api/notes?path=c.html", nil)
	expect(t, firstOf(a.save(t, "c.html", "# two", nil)), 200)
	rec, _ := a.do(t, "POST", "/api/trash/restore", map[string]any{"id": out["trashId"]})
	expect(t, rec, 409)
	rec, _ = a.do(t, "POST", "/api/trash/restore", map[string]any{"id": out["trashId"], "path": "c-restored.html"})
	expect(t, rec, 200)
}

func TestRevisionsCoalesceAndSurviveDeletion(t *testing.T) {
	a := newTestApp(t)
	expect(t, firstOf(a.save(t, "r.html", "# v0", nil)), 200)
	for i := 1; i <= 6; i++ {
		expect(t, firstOf(a.save(t, "r.html", "# v"+string(rune('0'+i)), nil)), 200)
	}
	revs, err := a.db.Revisions(t.Context(), "r.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 1 {
		t.Fatalf("rapid autosaves must coalesce into one revision, got %d", len(revs))
	}
	rec, out := a.do(t, "POST", "/api/notes/restore", map[string]any{"path": "r.html", "revisionId": revs[0].ID})
	expect(t, rec, 200)
	if out["content"] != "# v0" {
		t.Fatalf("restored content = %v", out["content"])
	}
	revs, _ = a.db.Revisions(t.Context(), "r.html")
	if len(revs) != 2 {
		t.Fatalf("restore must preserve the replaced version, got %d revisions", len(revs))
	}
	a.do(t, "DELETE", "/api/notes?path=r.html", nil)
	revs, _ = a.db.Revisions(t.Context(), "r.html")
	if len(revs) != 2 {
		t.Fatalf("deleting a note must keep its revisions, got %d", len(revs))
	}
}

func TestUpdatedAtFollowsFileModTime(t *testing.T) {
	a := newTestApp(t)
	p := filepath.Join(a.vaultDir, "old.html")
	if err := os.WriteFile(p, []byte("<html><title>Old</title><body>x</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-90 * 24 * time.Hour).UTC().Truncate(time.Second)
	if err := os.Chtimes(p, past, past); err != nil {
		t.Fatal(err)
	}
	if err := indexing.IndexFile(a.root, a.db, "old.html"); err != nil {
		t.Fatal(err)
	}
	d, err := a.db.Details(t.Context(), "old.html")
	if err != nil {
		t.Fatal(err)
	}
	got, err := time.Parse(time.RFC3339Nano, d.UpdatedAt)
	if err != nil || !got.Equal(past) {
		t.Fatalf("updatedAt = %q, want %s (err=%v)", d.UpdatedAt, past, err)
	}
}

func TestRenameConflictAndAtomicity(t *testing.T) {
	a := newTestApp(t)
	expect(t, firstOf(a.save(t, "x.html", "# X", nil)), 200)
	expect(t, firstOf(a.save(t, "y.html", "# Y", nil)), 200)
	rec, _ := a.do(t, "POST", "/api/notes/rename", map[string]any{"oldPath": "x.html", "newPath": "y.html"})
	if rec.Code != 409 && rec.Code != 400 {
		t.Fatalf("rename over existing note must fail, got %d", rec.Code)
	}
	raw, _ := os.ReadFile(filepath.Join(a.vaultDir, "y.html"))
	if !strings.Contains(string(raw), "Y") {
		t.Fatal("existing destination was modified")
	}
	rec, _ = a.do(t, "POST", "/api/notes/rename", map[string]any{"oldPath": "x.html", "newPath": "sub/z.html"})
	expect(t, rec, 200)
	if _, err := os.Stat(filepath.Join(a.vaultDir, "x.html")); !os.IsNotExist(err) {
		t.Fatal("source should be gone after rename")
	}
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func TestExportImportRoundTrip(t *testing.T) {
	src := newTestApp(t)
	expect(t, firstOf(src.save(t, "Notes/one.html", "# One [[two]]", nil)), 200)
	expect(t, firstOf(src.save(t, "two.html", "# Two", nil)), 200)
	if err := os.WriteFile(filepath.Join(src.vaultDir, "fig.png"), []byte("\x89PNG-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	src.do(t, "DELETE", "/api/notes?path=two.html", nil)
	expect(t, firstOf(src.save(t, "two.html", "# Two again", nil)), 200)
	rec, _ := src.do(t, "GET", "/api/export", nil)
	expect(t, rec, 200)
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
		if strings.HasPrefix(f.Name, ".sefirot") {
			t.Fatalf("export leaked reserved entry %s", f.Name)
		}
	}
	for _, want := range []string{"Notes/one.html", "two.html", "fig.png"} {
		if !names[want] {
			t.Fatalf("export missing %s: %v", want, names)
		}
	}
	dst := newTestApp(t)
	rec, out := dst.do(t, "POST", "/api/import", rec.Body.Bytes())
	expect(t, rec, 200)
	if int(out["count"].(float64)) != 3 {
		t.Fatalf("import report: %v", out)
	}
	rec, note := dst.do(t, "GET", "/api/notes?path=Notes/one.html", nil)
	expect(t, rec, 200)
	if note["content"] != "# One [[two]]" {
		t.Fatalf("markdown source lost in round trip: %v", note["content"])
	}
	raw, _ := os.ReadFile(filepath.Join(dst.vaultDir, "fig.png"))
	if string(raw) != "\x89PNG-bytes" {
		t.Fatal("binary asset corrupted")
	}
	rec, out = dst.do(t, "POST", "/api/import", zipOf(t, map[string]string{"two.html": "<html>overwrite</html>"}))
	expect(t, rec, 200)
	if int(out["count"].(float64)) != 0 || len(out["skipped"].([]any)) != 1 {
		t.Fatalf("import must not overwrite: %v", out)
	}
}

func TestImportRejectsUnsafeArchivesWithoutSideEffects(t *testing.T) {
	a := newTestApp(t)
	for name, files := range map[string]map[string]string{
		"traversal": {"ok.html": "<html>ok</html>", "../evil.html": "x"},
		"absolute":  {"/etc/evil.html": "x"},
		"backslash": {`a\b.html`: "x"},
		"reserved":  {".sefirot/trash/x/content": "x"},
	} {
		rec, _ := a.do(t, "POST", "/api/import", zipOf(t, files))
		if rec.Code != 400 {
			t.Fatalf("%s: status = %d, want 400", name, rec.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(a.vaultDir, "ok.html")); !os.IsNotExist(err) {
		t.Fatal("a rejected archive must not write any of its files")
	}
	if _, err := os.Stat(filepath.Join(a.dir, "evil.html")); !os.IsNotExist(err) {
		t.Fatal("traversal escaped the vault")
	}
}

func TestImportCountsActualBytesNotHeaders(t *testing.T) {
	a := newTestApp(t)
	old := maxEntryBytes
	maxEntryBytes = 1024
	defer func() { maxEntryBytes = old }()
	rec, _ := a.do(t, "POST", "/api/import", zipOf(t, map[string]string{"big.html": strings.Repeat("a", 4096)}))
	if rec.Code != 413 {
		t.Fatalf("oversized entry must be rejected, got %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(a.vaultDir, "big.html")); !os.IsNotExist(err) {
		t.Fatal("oversized entry was written")
	}
}

func TestCrossOriginMutationBlocked(t *testing.T) {
	a := newTestApp(t)
	req := httptest.NewRequest("POST", "http://127.0.0.1:43999/api/notes", strings.NewReader(`{"path":"z.html","content":"x","isMarkdown":true}`))
	req.Host = "127.0.0.1:43999"
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("cross-origin POST must be blocked, got %d", rec.Code)
	}
	req = httptest.NewRequest("GET", "http://evil.example:43999/api/tree", nil)
	rec = httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("foreign Host header must be blocked (DNS rebinding), got %d", rec.Code)
	}
}

func firstOf(rec *httptest.ResponseRecorder, _ map[string]any) *httptest.ResponseRecorder { return rec }

func TestSettingsPersistAndValidate(t *testing.T) {
	a := newTestApp(t)
	rec, got := a.do(t, "GET", "/api/settings", nil)
	expect(t, rec, 200)
	if got["theme"] != "dark" {
		t.Fatalf("default theme = %v", got["theme"])
	}
	good := map[string]any{"theme": "light", "tabs": []string{"a.html"}, "researchDocs": []string{"a.html"}, "macros": map[string]any{"R": "\\mathbb{R}", "norm": []any{"\\left\\lVert #1 \\right\\rVert", 1}}}
	rec, _ = a.do(t, "PUT", "/api/settings", good)
	expect(t, rec, 200)
	_, got = a.do(t, "GET", "/api/settings", nil)
	if got["theme"] != "light" || len(got["researchDocs"].([]any)) != 1 {
		t.Fatalf("settings not persisted: %v", got)
	}
	for name, bad := range map[string]map[string]any{
		"theme":   {"theme": "neon"},
		"macro":   {"theme": "dark", "macros": map[string]any{"x": 5}},
		"arity":   {"theme": "dark", "macros": map[string]any{"x": []any{"y", 12}}},
		"unknown": {"theme": "dark", "surprise": true},
	} {
		rec, _ = a.do(t, "PUT", "/api/settings", bad)
		if rec.Code != 400 {
			t.Fatalf("%s: status = %d, want 400", name, rec.Code)
		}
	}
}

func TestIntegrityReport(t *testing.T) {
	a := newTestApp(t)
	html := func(links ...string) string {
		s := "<html><title>t</title><body>"
		for _, l := range links {
			s += `<a data-wikilink="` + l + `">x</a>`
		}
		return s + "</body></html>"
	}
	files := map[string]string{
		"hub.html":    html("leaf", "missing", "dup"),
		"leaf.html":   html(),
		"A/dup.html":  html(),
		"B/dup.html":  html(),
		"lonely.html": html(),
	}
	for p, c := range files {
		full := filepath.Join(a.vaultDir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(c), 0o644)
		if err := indexing.IndexFile(a.root, a.db, p); err != nil {
			t.Fatal(err)
		}
	}
	rec, out := a.do(t, "GET", "/api/integrity", nil)
	expect(t, rec, 200)
	if len(out["brokenLinks"].([]any)) != 1 || len(out["ambiguousLinks"].([]any)) != 1 {
		t.Fatalf("report = %v", out)
	}
	orph := out["orphans"].([]any)
	if len(orph) != 1 || orph[0] != "lonely.html" {
		t.Fatalf("orphans = %v", orph)
	}
}

func TestVendorAssetsReachableFromSandboxedPreview(t *testing.T) {
	a := newTestApp(t)
	req := httptest.NewRequest("GET", "http://127.0.0.1:43999/vendor/none.woff", nil)
	req.Host = "127.0.0.1:43999"
	req.Header.Set("Origin", "null")
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	if rec.Code == 403 {
		t.Fatal("opaque-origin preview must be able to request vendor assets")
	}
	req = httptest.NewRequest("GET", "http://127.0.0.1:43999/api/tree", nil)
	req.Host = "127.0.0.1:43999"
	req.Header.Set("Origin", "null")
	rec = httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("API must stay closed to opaque origins, got %d", rec.Code)
	}
	rec, _ = a.do(t, "GET", "/visual-system.css", nil)
	expect(t, rec, 200)
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("stylesheet content type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestNewEmptyNoteStaysMarkdownAndRenders(t *testing.T) {
	a := newTestApp(t)
	rec, note := a.save(t, "fresh.html", "", map[string]any{"createOnly": true})
	expect(t, rec, 200)
	if note["isMarkdown"] != true {
		t.Fatalf("empty new note must report markdown mode: %v", note["isMarkdown"])
	}
	rec, note = a.do(t, "GET", "/api/notes?path=fresh.html", nil)
	expect(t, rec, 200)
	if note["isMarkdown"] != true {
		t.Fatal("empty note reopened as raw HTML")
	}
	rec, note = a.save(t, "fresh.html", "# Heading\n\ntext", nil)
	expect(t, rec, 200)
	if !strings.Contains(note["renderedHtml"].(string), "<h1") {
		t.Fatalf("markdown not rendered: %v", note["renderedHtml"])
	}
}

func TestWikiLinksResolveByTitleAndAlias(t *testing.T) {
	a := newTestApp(t)
	expect(t, firstOf(a.save(t, "notes/2024-quantum.html", "---\naliases: [QM, Quantum Mechanics]\ntags: [physics, draft]\ntype: concept\n---\n# Quantum Basics\n\nbody", nil)), 200)
	expect(t, firstOf(a.save(t, "A/dup.html", "# Dup A", nil)), 200)
	expect(t, firstOf(a.save(t, "B/dup.html", "# Dup B", nil)), 200)
	expect(t, firstOf(a.save(t, "hub.html", "[[Quantum Basics]] [[qm]] [[Quantum Mechanics]] [[dup]] [[nothing]]", nil)), 200)
	for target, want := range map[string]string{"Quantum Basics": "notes/2024-quantum.html", "QM": "notes/2024-quantum.html", "quantum mechanics": "notes/2024-quantum.html", "2024-quantum": "notes/2024-quantum.html", "notes/2024-quantum": "notes/2024-quantum.html"} {
		rec, out := a.do(t, "GET", "/api/resolve?target="+strings.ReplaceAll(target, " ", "%20"), nil)
		expect(t, rec, 200)
		if out["path"] != want {
			t.Errorf("resolve %q = %v, want %s", target, out["path"], want)
		}
	}
	rec, out := a.do(t, "GET", "/api/resolve?target=dup", nil)
	expect(t, rec, 200)
	if out["path"] != "" || len(out["candidates"].([]any)) != 2 {
		t.Fatalf("ambiguous target must list candidates, not guess: %v", out)
	}
	rec, out = a.do(t, "GET", "/api/resolve?target=nothing", nil)
	if out["path"] != "" || len(out["candidates"].([]any)) != 0 {
		t.Fatalf("missing target: %v", out)
	}
	rec, note := a.do(t, "GET", "/api/notes?path=notes/2024-quantum.html", nil)
	expect(t, rec, 200)
	if bl := note["backlinks"].([]any); len(bl) != 1 {
		t.Fatalf("backlinks via title/alias = %v", bl)
	}
	tags, _ := note["tags"].([]any)
	if note["docType"] != "concept" && note["type"] != "concept" {
		t.Logf("note payload keys: %v", note)
	}
	_ = tags
	rec, out = a.do(t, "GET", "/api/integrity", nil)
	expect(t, rec, 200)
	if len(out["brokenLinks"].([]any)) != 1 || len(out["ambiguousLinks"].([]any)) != 1 {
		t.Fatalf("integrity = %v", out)
	}
	rec, results := a.do(t, "GET", "/api/search?q=tag:physics", nil)
	expect(t, rec, 200)
	_ = results
	if !strings.Contains(rec.Body.String(), "2024-quantum.html") {
		t.Fatalf("front matter tags must be searchable: %s", rec.Body.String())
	}
	rec, note = a.do(t, "GET", "/api/notes?path=notes/2024-quantum.html", nil)
	if !strings.HasPrefix(note["content"].(string), "---\naliases:") {
		t.Fatal("front matter must stay in the editable source")
	}
	if strings.Contains(note["renderedHtml"].(string), "aliases: [QM") {
		t.Fatal("front matter must not be rendered into the page")
	}
}

func TestSearchFindsMathNotationAcrossForms(t *testing.T) {
	a := newTestApp(t)
	expect(t, firstOf(a.save(t, "grad.html", "# Vector calculus\n\nThe field $\\nabla f$ and the Greek letter α appear here.", nil)), 200)
	expect(t, firstOf(a.save(t, "other.html", "# Cooking\n\nNothing mathematical.", nil)), 200)
	for _, q := range []string{"∇", `\nabla`, "nabla", "gradient", "alpha", "α", "∇f"} {
		rec, _ := a.do(t, "GET", "/api/search?q="+url.QueryEscape(q), nil)
		expect(t, rec, 200)
		if !strings.Contains(rec.Body.String(), "grad.html") || strings.Contains(rec.Body.String(), "other.html") {
			t.Errorf("query %q returned %s", q, rec.Body.String())
		}
	}
}
