package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type ResearchPaper struct {
	ID             int64    `json:"id"`
	Title          string   `json:"title"`
	Authors        []string `json:"authors"`
	Abstract       string   `json:"abstract"`
	DOI            string   `json:"doi"`
	Publication    string   `json:"publication"`
	PublishedAt    string   `json:"publishedAt"`
	URL            string   `json:"url"`
	Keywords       []string `json:"keywords"`
	SourceFile     string   `json:"sourceFile"`
	ReadingStatus  string   `json:"readingStatus"`
	PersonalNotes  string   `json:"personalNotes"`
	MetadataOrigin string   `json:"metadataOrigin"`
	EntryType      string   `json:"entryType,omitempty"`
	CiteKey        string   `json:"citeKey,omitempty"`
	SourceRecord   string   `json:"sourceRecord,omitempty"`
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt"`
}

type ResearchCitation struct {
	ID           int64  `json:"id"`
	PaperID      int64  `json:"paperId"`
	CitedPaperID int64  `json:"citedPaperId,omitempty"`
	CitationText string `json:"citationText"`
	Locator      string `json:"locator"`
	Origin       string `json:"origin"`
	CreatedAt    string `json:"createdAt"`
}

type ResearchAnnotation struct {
	ID        int64  `json:"id"`
	PaperID   int64  `json:"paperId"`
	Page      int    `json:"page"`
	Quote     string `json:"quote"`
	Note      string `json:"note"`
	Origin    string `json:"origin"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func (d *Database) ResearchSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS research_papers (id INTEGER PRIMARY KEY AUTOINCREMENT,title TEXT NOT NULL,authors_json TEXT NOT NULL DEFAULT '[]',abstract TEXT NOT NULL DEFAULT '',doi TEXT NOT NULL DEFAULT '',publication TEXT NOT NULL DEFAULT '',published_at TEXT NOT NULL DEFAULT '',url TEXT NOT NULL DEFAULT '',keywords_json TEXT NOT NULL DEFAULT '[]',source_file TEXT NOT NULL DEFAULT '',reading_status TEXT NOT NULL DEFAULT 'unread',personal_notes TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_research_doi ON research_papers(doi) WHERE doi <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_research_updated ON research_papers(updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS research_citations (id INTEGER PRIMARY KEY AUTOINCREMENT,paper_id INTEGER NOT NULL REFERENCES research_papers(id) ON DELETE CASCADE,cited_paper_id INTEGER REFERENCES research_papers(id) ON DELETE SET NULL,citation_text TEXT NOT NULL,locator TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_research_citations_paper ON research_citations(paper_id)`,
		`CREATE TABLE IF NOT EXISTS research_annotations (id INTEGER PRIMARY KEY AUTOINCREMENT,paper_id INTEGER NOT NULL REFERENCES research_papers(id) ON DELETE CASCADE,page INTEGER NOT NULL DEFAULT 0,quote TEXT NOT NULL DEFAULT '',note TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_research_annotations_paper ON research_annotations(paper_id,page)`,
	}
	for _, stmt := range stmts {
		if _, err := d.Writer.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

const paperColumns = `id,title,authors_json,abstract,doi,publication,published_at,url,keywords_json,source_file,reading_status,personal_notes,metadata_origin,entry_type,cite_key,source_record,created_at,updated_at`

var validOrigins = map[string]bool{"manual": true, "bibtex": true, "ris": true, "crossref": true}

func validOrigin(o string) bool { return validOrigins[strings.TrimSuffix(o, "-edited")] }

// SaveResearchPaper creates or updates a paper. Provenance (origin, entry type,
// cite key and the verbatim source record) is only set on creation and is never
// overwritten by an update; editing an imported paper marks its origin as edited.
func (d *Database) SaveResearchPaper(ctx context.Context, p ResearchPaper) (ResearchPaper, error) {
	p.Title = strings.TrimSpace(p.Title)
	if p.Title == "" {
		return p, errors.New("title is required")
	}
	if p.ReadingStatus == "" {
		p.ReadingStatus = "unread"
	}
	switch p.ReadingStatus {
	case "unread", "reading", "read", "archived":
	default:
		return p, errors.New("invalid reading status")
	}
	authors, err := json.Marshal(cleanStrings(p.Authors))
	if err != nil {
		return p, err
	}
	keywords, err := json.Marshal(cleanStrings(p.Keywords))
	if err != nil {
		return p, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if p.ID == 0 {
		if p.MetadataOrigin == "" {
			p.MetadataOrigin = "manual"
		}
		if !validOrigin(p.MetadataOrigin) {
			return p, errors.New("invalid metadata origin")
		}
		p.CreatedAt, p.UpdatedAt = now, now
		res, err := d.Writer.ExecContext(ctx, `INSERT INTO research_papers(title,authors_json,abstract,doi,publication,published_at,url,keywords_json,source_file,reading_status,personal_notes,metadata_origin,entry_type,cite_key,source_record,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.Title, string(authors), p.Abstract, strings.TrimSpace(p.DOI), p.Publication, p.PublishedAt, p.URL, string(keywords), p.SourceFile, p.ReadingStatus, p.PersonalNotes, p.MetadataOrigin, p.EntryType, p.CiteKey, p.SourceRecord, p.CreatedAt, p.UpdatedAt)
		if err != nil {
			return p, err
		}
		p.ID, err = res.LastInsertId()
		return p, err
	}
	existing, err := d.ResearchPaper(ctx, p.ID)
	if err != nil {
		return p, err
	}
	origin := existing.MetadataOrigin
	if origin != "manual" && !strings.HasSuffix(origin, "-edited") && bibliographicChange(existing, p) {
		origin += "-edited"
	}
	p.UpdatedAt = now
	res, err := d.Writer.ExecContext(ctx, `UPDATE research_papers SET title=?,authors_json=?,abstract=?,doi=?,publication=?,published_at=?,url=?,keywords_json=?,source_file=?,reading_status=?,personal_notes=?,metadata_origin=?,updated_at=? WHERE id=?`,
		p.Title, string(authors), p.Abstract, strings.TrimSpace(p.DOI), p.Publication, p.PublishedAt, p.URL, string(keywords), p.SourceFile, p.ReadingStatus, p.PersonalNotes, origin, p.UpdatedAt, p.ID)
	if err != nil {
		return p, err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return p, sql.ErrNoRows
	}
	p.MetadataOrigin, p.EntryType, p.CiteKey, p.SourceRecord, p.CreatedAt = origin, existing.EntryType, existing.CiteKey, existing.SourceRecord, existing.CreatedAt
	return p, err
}

func bibliographicChange(a, b ResearchPaper) bool {
	join := func(v []string) string { return strings.Join(cleanStrings(v), "\x00") }
	return strings.TrimSpace(a.Title) != strings.TrimSpace(b.Title) || join(a.Authors) != join(b.Authors) || a.Abstract != b.Abstract ||
		strings.TrimSpace(a.DOI) != strings.TrimSpace(b.DOI) || a.Publication != b.Publication || a.PublishedAt != b.PublishedAt || a.URL != b.URL
}

// FindPaperByDOI returns the paper with the given DOI, if any.
func (d *Database) FindPaperByDOI(ctx context.Context, doi string) (ResearchPaper, bool, error) {
	doi = strings.TrimSpace(doi)
	if doi == "" {
		return ResearchPaper{}, false, nil
	}
	p, err := scanResearchPaper(d.Reader.QueryRowContext(ctx, `SELECT `+paperColumns+` FROM research_papers WHERE doi=?`, doi))
	if err == sql.ErrNoRows {
		return ResearchPaper{}, false, nil
	}
	return p, err == nil, err
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func (d *Database) ResearchPapers(ctx context.Context) ([]ResearchPaper, error) {
	rows, err := d.Reader.QueryContext(ctx, `SELECT `+paperColumns+` FROM research_papers ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ResearchPaper{}
	for rows.Next() {
		p, err := scanResearchPaper(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *Database) ResearchPaper(ctx context.Context, id int64) (ResearchPaper, error) {
	return scanResearchPaper(d.Reader.QueryRowContext(ctx, `SELECT `+paperColumns+` FROM research_papers WHERE id=?`, id))
}

type scanner interface{ Scan(...any) error }

func scanResearchPaper(row scanner) (ResearchPaper, error) {
	var p ResearchPaper
	var authors, keywords string
	err := row.Scan(&p.ID, &p.Title, &authors, &p.Abstract, &p.DOI, &p.Publication, &p.PublishedAt, &p.URL, &keywords, &p.SourceFile, &p.ReadingStatus, &p.PersonalNotes, &p.MetadataOrigin, &p.EntryType, &p.CiteKey, &p.SourceRecord, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal([]byte(authors), &p.Authors)
	_ = json.Unmarshal([]byte(keywords), &p.Keywords)
	return p, nil
}

func (d *Database) DeleteResearchPaper(ctx context.Context, id int64) error {
	res, err := d.Writer.ExecContext(ctx, `DELETE FROM research_papers WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return err
}

func normalizeChildOrigin(o string) (string, error) {
	switch o {
	case "":
		return "manual", nil
	case "manual", "extracted", "ai":
		return o, nil
	}
	return "", errors.New("invalid origin")
}

func (d *Database) AddResearchCitation(ctx context.Context, c ResearchCitation) (ResearchCitation, error) {
	if strings.TrimSpace(c.CitationText) == "" {
		return c, errors.New("citation text is required")
	}
	var err error
	if c.Origin, err = normalizeChildOrigin(c.Origin); err != nil {
		return c, err
	}
	c.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	res, err := d.Writer.ExecContext(ctx, `INSERT INTO research_citations(paper_id,cited_paper_id,citation_text,locator,origin,created_at) VALUES(?,?,?,?,?,?)`, c.PaperID, nullID(c.CitedPaperID), c.CitationText, c.Locator, c.Origin, c.CreatedAt)
	if err != nil {
		return c, err
	}
	c.ID, err = res.LastInsertId()
	return c, err
}

func nullID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

func (d *Database) ResearchCitations(ctx context.Context, id int64) ([]ResearchCitation, error) {
	rows, err := d.Reader.QueryContext(ctx, `SELECT id,paper_id,COALESCE(cited_paper_id,0),citation_text,locator,origin,created_at FROM research_citations WHERE paper_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ResearchCitation{}
	for rows.Next() {
		var c ResearchCitation
		if err := rows.Scan(&c.ID, &c.PaperID, &c.CitedPaperID, &c.CitationText, &c.Locator, &c.Origin, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (d *Database) AddResearchAnnotation(ctx context.Context, a ResearchAnnotation) (ResearchAnnotation, error) {
	if strings.TrimSpace(a.Note) == "" {
		return a, errors.New("annotation note is required")
	}
	var err error
	if a.Origin, err = normalizeChildOrigin(a.Origin); err != nil {
		return a, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	a.CreatedAt = now
	a.UpdatedAt = now
	res, err := d.Writer.ExecContext(ctx, `INSERT INTO research_annotations(paper_id,page,quote,note,origin,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, a.PaperID, a.Page, a.Quote, a.Note, a.Origin, now, now)
	if err != nil {
		return a, err
	}
	a.ID, err = res.LastInsertId()
	return a, err
}

func (d *Database) ResearchAnnotations(ctx context.Context, id int64) ([]ResearchAnnotation, error) {
	rows, err := d.Reader.QueryContext(ctx, `SELECT id,paper_id,page,quote,note,origin,created_at,updated_at FROM research_annotations WHERE paper_id=? ORDER BY page,id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ResearchAnnotation{}
	for rows.Next() {
		var a ResearchAnnotation
		if err := rows.Scan(&a.ID, &a.PaperID, &a.Page, &a.Quote, &a.Note, &a.Origin, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type ResearchRecord struct {
	Version     int                  `json:"version"`
	Paper       ResearchPaper        `json:"paper"`
	Citations   []ResearchCitation   `json:"citations"`
	Annotations []ResearchAnnotation `json:"annotations"`
}

func (d *Database) ResearchRecord(ctx context.Context, id int64) (ResearchRecord, error) {
	p, err := d.ResearchPaper(ctx, id)
	if err != nil {
		return ResearchRecord{}, err
	}
	c, err := d.ResearchCitations(ctx, id)
	if err != nil {
		return ResearchRecord{}, err
	}
	a, err := d.ResearchAnnotations(ctx, id)
	if err != nil {
		return ResearchRecord{}, err
	}
	return ResearchRecord{Version: 1, Paper: p, Citations: c, Annotations: a}, nil
}

func (d *Database) DeleteResearchCitation(ctx context.Context, paperID, id int64) error {
	return deleteOne(ctx, d, `DELETE FROM research_citations WHERE id=? AND paper_id=?`, id, paperID)
}

func (d *Database) DeleteResearchAnnotation(ctx context.Context, paperID, id int64) error {
	return deleteOne(ctx, d, `DELETE FROM research_annotations WHERE id=? AND paper_id=?`, id, paperID)
}

func deleteOne(ctx context.Context, d *Database, query string, args ...any) error {
	res, err := d.Writer.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return err
}

// RestoreResearchRecord loads a vault-stored record into the index when the paper
// is absent or the stored copy is newer. It reports whether the index changed.
func (d *Database) RestoreResearchRecord(ctx context.Context, rec ResearchRecord) (bool, error) {
	p := rec.Paper
	if p.ID < 1 || strings.TrimSpace(p.Title) == "" {
		return false, errors.New("invalid research record")
	}
	tx, err := d.Writer.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT updated_at FROM research_papers WHERE id=?`, p.ID).Scan(&existing)
	if err == nil && existing >= p.UpdatedAt {
		return false, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	authors, _ := json.Marshal(cleanStrings(p.Authors))
	keywords, _ := json.Marshal(cleanStrings(p.Keywords))
	if _, err = tx.ExecContext(ctx, `INSERT INTO research_papers(id,title,authors_json,abstract,doi,publication,published_at,url,keywords_json,source_file,reading_status,personal_notes,metadata_origin,entry_type,cite_key,source_record,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET title=excluded.title,authors_json=excluded.authors_json,abstract=excluded.abstract,doi=excluded.doi,publication=excluded.publication,published_at=excluded.published_at,url=excluded.url,keywords_json=excluded.keywords_json,source_file=excluded.source_file,reading_status=excluded.reading_status,personal_notes=excluded.personal_notes,metadata_origin=excluded.metadata_origin,entry_type=excluded.entry_type,cite_key=excluded.cite_key,source_record=excluded.source_record,updated_at=excluded.updated_at`,
		p.ID, p.Title, string(authors), p.Abstract, strings.TrimSpace(p.DOI), p.Publication, p.PublishedAt, p.URL, string(keywords), p.SourceFile, p.ReadingStatus, p.PersonalNotes, originOrManual(p.MetadataOrigin), p.EntryType, p.CiteKey, p.SourceRecord, p.CreatedAt, p.UpdatedAt); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM research_citations WHERE paper_id=?`, p.ID); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM research_annotations WHERE paper_id=?`, p.ID); err != nil {
		return false, err
	}
	for _, c := range rec.Citations {
		cited := c.CitedPaperID
		if cited > 0 {
			var one int
			if tx.QueryRowContext(ctx, `SELECT 1 FROM research_papers WHERE id=?`, cited).Scan(&one) != nil {
				cited = 0
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO research_citations(id,paper_id,cited_paper_id,citation_text,locator,origin,created_at) VALUES(?,?,?,?,?,?,?)`, c.ID, p.ID, nullID(cited), c.CitationText, c.Locator, originOrManual(c.Origin), c.CreatedAt); err != nil {
			return false, err
		}
	}
	for _, a := range rec.Annotations {
		if _, err = tx.ExecContext(ctx, `INSERT INTO research_annotations(id,paper_id,page,quote,note,origin,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, a.ID, p.ID, a.Page, a.Quote, a.Note, originOrManual(a.Origin), a.CreatedAt, a.UpdatedAt); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

func originOrManual(o string) string {
	if o == "" {
		return "manual"
	}
	return o
}
