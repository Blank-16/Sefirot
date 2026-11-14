package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"path"
	"strings"
)

type NoteDetails struct {
	Description   string            `json:"description"`
	Summary       string            `json:"summary"`
	SummaryShort  string            `json:"summaryShort"`
	SummaryDetail string            `json:"summaryDetail"`
	Headings      []DocumentHeading `json:"headings"`
	DocType       string            `json:"type"`
	Tags          []string          `json:"tags"`
	ContentHash   string            `json:"contentHash"`
	UpdatedAt     string            `json:"updatedAt"`
}

func (d *Database) Details(ctx context.Context, notePath string) (NoteDetails, error) {
	var out NoteDetails
	var tags, headings string
	err := d.Reader.QueryRowContext(ctx, `SELECT description,summary,summary_short,summary_detail,headings_json,doc_type,tags_json,content_hash,updated_at FROM notes WHERE path=?`, notePath).Scan(&out.Description, &out.Summary, &out.SummaryShort, &out.SummaryDetail, &headings, &out.DocType, &tags, &out.ContentHash, &out.UpdatedAt)
	if err != nil {
		return out, err
	}
	_ = json.Unmarshal([]byte(tags), &out.Tags)
	_ = json.Unmarshal([]byte(headings), &out.Headings)
	return out, nil
}
func (d *Database) HasHash(ctx context.Context, notePath, hash string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var existing string
	err := d.Reader.QueryRowContext(ctx, `SELECT content_hash FROM notes WHERE path=?`, notePath).Scan(&existing)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return existing == hash, nil
}
func (d *Database) PruneMissing(ctx context.Context, existing map[string]struct{}) error {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := d.Reader.QueryContext(ctx, `SELECT path FROM notes`)
	if err != nil {
		return err
	}
	var missing []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		if _, ok := existing[p]; !ok {
			missing = append(missing, p)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, p := range missing {
		if err := d.Delete(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func (d *Database) Close() {
	if d.Reader != nil {
		_ = d.Reader.Close()
	}
	if d.Writer != nil {
		_ = d.Writer.Close()
	}
}

func normalizeSlug(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, `\`, `/`))
	if strings.EqualFold(path.Ext(s), ".html") {
		s = strings.TrimSuffix(s, path.Ext(s))
	}
	return strings.ToLower(path.Clean(s))
}
