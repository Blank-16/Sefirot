package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"path"
	"sefirot/internal/mathsearch"
	"sort"
	"strings"
	"time"
)

func (d *Database) Upsert(ctx context.Context, n IndexedNote) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if n.DocType == "" {
		n.DocType = "note"
	}
	n.DocType = strings.ToLower(strings.TrimSpace(n.DocType))
	if n.Summary == "" {
		n.Summary = SummarizeDocument(n.PlainText)
	}
	if n.SummaryShort == "" {
		n.SummaryShort = SummarizeWords(n.Summary, 18)
	}
	if n.SummaryDetail == "" {
		n.SummaryDetail = SummarizeWords(n.PlainText, 120)
	}
	tagSet := map[string]bool{}
	cleanTags := make([]string, 0, len(n.Tags))
	for _, tag := range n.Tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" && !tagSet[tag] {
			tagSet[tag] = true
			cleanTags = append(cleanTags, tag)
		}
	}
	sort.Strings(cleanTags)
	n.Tags = cleanTags
	tagsJSON, err := json.Marshal(n.Tags)
	if err != nil {
		return err
	}
	headingsJSON, err := json.Marshal(n.Headings)
	if err != nil {
		return err
	}
	tx, err := d.Writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	var oldHash, oldHTML string
	err = tx.QueryRowContext(ctx, `SELECT content_hash,content_html FROM notes WHERE path=?`, n.Path).Scan(&oldHash, &oldHTML)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && oldHash != "" && n.ContentHash != "" && oldHash != n.ContentHash && oldHTML != "" {
		if err = insertRevision(ctx, tx, n.Path, oldHash, oldHTML, now, RevisionMinInterval); err != nil {
			return err
		}
	}
	updatedAt := n.UpdatedAt
	if updatedAt == "" {
		updatedAt = now.Format(time.RFC3339Nano)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO notes(path,title,plain_text,summary,summary_short,summary_detail,headings_json,description,doc_type,tags_json,content_hash,content_html,updated_at,math_text)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET title=excluded.title,plain_text=excluded.plain_text,summary=excluded.summary,summary_short=excluded.summary_short,summary_detail=excluded.summary_detail,headings_json=excluded.headings_json,description=excluded.description,doc_type=excluded.doc_type,tags_json=excluded.tags_json,content_hash=excluded.content_hash,content_html=excluded.content_html,updated_at=excluded.updated_at,math_text=excluded.math_text`,
		n.Path, n.Title, n.PlainText, n.Summary, n.SummaryShort, n.SummaryDetail, string(headingsJSON), n.Description, n.DocType, string(tagsJSON), n.ContentHash, n.ContentHTML, updatedAt, mathsearch.Tokens(n.PlainText))
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM links WHERE source_path=?`, n.Path); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM relationships WHERE source_path=?`, n.Path); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM note_aliases WHERE path=?`, n.Path); err != nil {
		return err
	}
	for _, alias := range n.Aliases {
		if slug := normalizeSlug(alias); slug != "" && slug != "." {
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO note_aliases(path,alias_slug) VALUES(?,?)`, n.Path, slug); err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM note_tags WHERE path=?`, n.Path); err != nil {
		return err
	}
	for _, target := range n.Links {
		slug := normalizeSlug(target)
		if slug == "" || slug == "." {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO links(source_path,target_slug) VALUES(?,?)`, n.Path, slug); err != nil {
			return err
		}
	}
	for _, rel := range n.Relationships {
		slug := normalizeSlug(rel.Target)
		typ := strings.TrimSpace(rel.Type)
		if typ == "" {
			typ = "related-to"
		}
		if slug == "" || slug == "." {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO relationships(source_path,target_slug,relation_type) VALUES(?,?,?)`, n.Path, slug, typ); err != nil {
			return err
		}
	}
	for _, tag := range n.Tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO note_tags(path,tag) VALUES(?,?)`, n.Path, tag); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *Database) Delete(ctx context.Context, notePath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := d.Writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"links", "relationships", "note_tags", "note_aliases", "embeddings"} {
		if _, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE `+map[string]string{"links": "source_path", "relationships": "source_path", "note_tags": "path", "note_aliases": "path", "embeddings": "path"}[table]+`=?`, notePath); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM notes WHERE path=?`, notePath); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *Database) Rename(ctx context.Context, oldPath, newPath string) error {
	tx, err := d.Writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE notes SET path=? WHERE path=?`, newPath, oldPath); err != nil {
		return err
	}
	for _, table := range []string{"links", "relationships"} {
		if _, err = tx.ExecContext(ctx, `UPDATE `+table+` SET source_path=? WHERE source_path=?`, newPath, oldPath); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE note_tags SET path=? WHERE path=?`, newPath, oldPath); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE note_aliases SET path=? WHERE path=?`, newPath, oldPath); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE revisions SET path=? WHERE path=?`, newPath, oldPath); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE embeddings SET path=? WHERE path=?`, newPath, oldPath); err != nil {
		return err
	}
	oldFull := normalizeSlug(strings.TrimSuffix(oldPath, path.Ext(oldPath)))
	newFull := normalizeSlug(strings.TrimSuffix(newPath, path.Ext(newPath)))
	oldBase := normalizeSlug(strings.TrimSuffix(path.Base(oldPath), path.Ext(oldPath)))
	newBase := normalizeSlug(strings.TrimSuffix(path.Base(newPath), path.Ext(newPath)))
	moveSlug := func(oldSlug, newSlug string) error {
		if oldSlug == newSlug {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO links(source_path,target_slug) SELECT source_path,? FROM links WHERE target_slug=?`, newSlug, oldSlug); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM links WHERE target_slug=?`, oldSlug); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO relationships(source_path,target_slug,relation_type) SELECT source_path,?,relation_type FROM relationships WHERE target_slug=?`, newSlug, oldSlug); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM relationships WHERE target_slug=?`, oldSlug)
		return err
	}
	if err := moveSlug(oldFull, newFull); err != nil {
		return err
	}
	if oldBase != oldFull {
		if err := moveSlug(oldBase, newBase); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RevisionMinInterval coalesces rapid autosaves: a new snapshot of the previous
// content is stored only when the newest snapshot is at least this old.
const RevisionMinInterval = 5 * time.Minute

const maxRevisionsPerPath = 50

type execQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func insertRevision(ctx context.Context, q execQuerier, notePath, hash, html string, now time.Time, minInterval time.Duration) error {
	var newestAt, newestHash string
	err := q.QueryRowContext(ctx, `SELECT created_at,content_hash FROM revisions WHERE path=? ORDER BY id DESC LIMIT 1`, notePath).Scan(&newestAt, &newestHash)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		if newestHash == hash {
			return nil
		}
		if at, perr := time.Parse(time.RFC3339Nano, newestAt); perr == nil && now.Sub(at) < minInterval {
			return nil
		}
	}
	if _, err = q.ExecContext(ctx, `INSERT INTO revisions(path,content_hash,content_html,created_at) VALUES(?,?,?,?)`, notePath, hash, html, now.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `DELETE FROM revisions WHERE path=? AND id NOT IN (SELECT id FROM revisions WHERE path=? ORDER BY id DESC LIMIT ?)`, notePath, notePath, maxRevisionsPerPath)
	return err
}

// SnapshotRevision stores the currently indexed content as a revision regardless
// of the coalescing interval. It is used before explicit, user-initiated rewrites.
func (d *Database) SnapshotRevision(ctx context.Context, notePath string) error {
	tx, err := d.Writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var hash, html string
	err = tx.QueryRowContext(ctx, `SELECT content_hash,content_html FROM notes WHERE path=?`, notePath).Scan(&hash, &html)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if hash == "" || html == "" {
		return nil
	}
	if err = insertRevision(ctx, tx, notePath, hash, html, time.Now().UTC(), 0); err != nil {
		return err
	}
	return tx.Commit()
}

// PruneOrphanRevisions drops revisions of notes that no longer exist and were
// last captured before cutoff. Deleted notes keep their history until then.
func (d *Database) PruneOrphanRevisions(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := d.Writer.ExecContext(ctx, `DELETE FROM revisions WHERE path NOT IN (SELECT path FROM notes) AND created_at < ?`, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
