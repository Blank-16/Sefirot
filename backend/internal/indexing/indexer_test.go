package indexing

import (
	"context"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sefirot/internal/database"
)

func TestExtractHTMLDetailsSkipsExecutableAndStyleText(t *testing.T) {
	raw := []byte(`<!doctype html><html><head><title>Systems Design</title><meta name="description" content="A compact systems overview."><meta name="vault:type" content="concept"><meta name="keywords" content="systems, architecture"></head><body><h1>Systems</h1><p>Replication and consensus.</p><script>secretToken()</script><style>.hidden{color:red}</style><a data-wikilink="Consensus" data-relation="depends-on">Consensus</a></body></html>`)
	title, plain, links, rels, meta, err := ExtractHTMLDetails(raw)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Systems Design" {
		t.Fatalf("title = %q", title)
	}
	if strings.Contains(plain, "secretToken") || strings.Contains(plain, "color:red") {
		t.Fatalf("executable/style text leaked into index: %q", plain)
	}
	if len(links) != 1 || links[0] != "Consensus" {
		t.Fatalf("links = %#v", links)
	}
	if len(rels) != 1 || rels[0].Type != "depends-on" {
		t.Fatalf("relationships = %#v", rels)
	}
	if meta.DocType != "concept" || len(meta.Tags) != 2 || meta.Description == "" || len(meta.Headings) != 1 || meta.Headings[0].Level != 1 {
		t.Fatalf("metadata = %#v", meta)
	}
}

func TestSafeRelativePath(t *testing.T) {
	for _, input := range []string{"../outside.html", "/etc/passwd", `C:\\Windows\\x.html`, "a/../../b.html"} {
		if _, err := SafeRelativePath(input); err == nil {
			t.Errorf("expected %q to be rejected", input)
		}
	}
	got, err := SafeRelativePath(`Engineering\\Systems.html`)
	if err != nil || got != "Engineering/Systems.html" {
		t.Fatalf("normalized path = %q, err=%v", got, err)
	}
}

func TestMarkdownSourceRoundTrip(t *testing.T) {
	source := "# Topic\n\nSee [[Other Note]]."
	rendered, links, err := RenderMarkdown(source)
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok := ExtractMarkdownSource([]byte(rendered))
	if !ok || decoded != source {
		t.Fatalf("source round trip failed: %q", decoded)
	}
	if len(links) != 1 || links[0] != "Other Note" {
		t.Fatalf("links = %#v", links)
	}
}

func TestSummarizeDocument(t *testing.T) {
	if got := database.SummarizeDocument("  "); got != "No summary available yet." {
		t.Fatalf("empty summary = %q", got)
	}
	words := strings.Repeat("word ", 60)
	got := database.SummarizeDocument(words)
	if len(strings.Fields(got)) != 42 {
		t.Fatalf("expected 42 words, got %d", len(strings.Fields(got)))
	}
}

func TestEmptyMarkdownNoteIsStillMarkdown(t *testing.T) {
	doc, _, err := RenderMarkdown("")
	if err != nil {
		t.Fatal(err)
	}
	source, ok := ExtractMarkdownSource([]byte(doc))
	if !ok || source != "" {
		t.Fatalf("empty markdown note lost its mode: ok=%v source=%q", ok, source)
	}
}

func TestMarkdownPreservesMathSource(t *testing.T) {
	cases := map[string][]string{
		"Inline $x_1 + y_1 = a*b$ done":                        {`$x_1 + y_1 = a*b$`},
		"Block:\n\n$$\n\\sum_{i=1}^n a_i\n$$\n":                {"$$\n\\sum_{i=1}^n a_i\n$$"},
		`Paren \(a_b\) and \[ c_d \]`:                          {`\(a_b\)`, `\[ c_d \]`},
		`Escaped \$5 stays`:                                    {`\$5`},
		"Matrix $\\begin{pmatrix}1&2\\\\3&4\\end{pmatrix}$ ok": {`$\begin{pmatrix}1&2\\3&4\end{pmatrix}$`},
	}
	for src, wants := range cases {
		doc, _, err := RenderMarkdown(src)
		if err != nil {
			t.Fatal(err)
		}
		body := doc[strings.Index(doc, "<body>"):]
		for _, want := range wants {
			if !strings.Contains(body, html.EscapeString(want)) {
				t.Errorf("source %q: math %q not preserved in %q", src, want, body)
			}
		}
		if strings.Contains(body, "<em>") || strings.Contains(body, "<strong>") {
			t.Errorf("source %q: math was parsed as emphasis: %q", src, body)
		}
	}
}

func TestCurrencyIsNotMath(t *testing.T) {
	doc, _, err := RenderMarkdown("It costs $5 and later $10 in *total*.")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc, "<em>total</em>") || !strings.Contains(doc, "$5 and later $10") {
		t.Fatalf("currency mangled: %s", doc)
	}
}

func TestUnterminatedMathFallsBack(t *testing.T) {
	doc, _, err := RenderMarkdown("Start $$ never closed\n\nnext *para*")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc, "<em>para</em>") {
		t.Fatalf("unterminated display math swallowed content: %s", doc)
	}
}

func TestForcedReindexRepopulatesDerivedData(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	db, err := database.OpenDatabase(filepath.Join(dir, "..", filepath.Base(dir)+".db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	doc, _, err := RenderMarkdown("---\naliases: [Alt Name]\n---\n# Heading\n\ntext")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "n.html"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RebuildIndex(root, db); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		var n int
		if err := db.Reader.QueryRow(`SELECT count(*) FROM note_aliases`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count() != 1 {
		t.Fatalf("aliases not indexed: %d", count())
	}
	db.Writer.Exec(`DELETE FROM note_aliases`)
	if err := RebuildIndex(root, db); err != nil {
		t.Fatal(err)
	}
	if count() != 0 {
		t.Fatal("an unchanged file must not be reindexed without the flag")
	}
	db.Writer.Exec(`INSERT OR REPLACE INTO meta(key,value) VALUES('reindex_required','1')`)
	if err := RebuildIndex(root, db); err != nil {
		t.Fatal(err)
	}
	if count() != 1 {
		t.Fatal("forced reindex must rebuild derived data")
	}
	if db.ReindexRequired(context.Background()) {
		t.Fatal("flag must clear after a clean reindex")
	}
}

func TestFrontMatterParsing(t *testing.T) {
	front, body := ParseFrontMatter("---\ntitle: \"A: B\"\naliases:\n  - one\n  - two\ntags: [x, \"y z\"]\n---\nbody")
	if body != "body" || front.first("title") != "A: B" || len(front.list("aliases")) != 2 || len(front.list("tags")) != 2 {
		t.Fatalf("parsed %v / %q", front, body)
	}
	for _, src := range []string{"no front matter", "---\nunterminated: yes\nbody", "---\n  bad indent: x\n---\nbody"} {
		if f, b := ParseFrontMatter(src); f != nil || b != src {
			t.Errorf("%q should pass through unchanged", src)
		}
	}
}
