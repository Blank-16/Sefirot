package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type migration struct {
	version int
	name    string
	apply   func(ctx context.Context, tx *sql.Tx) error
}

func execAll(statements ...string) func(context.Context, *sql.Tx) error {
	return func(ctx context.Context, tx *sql.Tx) error {
		for _, s := range statements {
			if _, err := tx.ExecContext(ctx, s); err != nil && !(strings.HasPrefix(s, "ALTER TABLE") && duplicateColumn(err)) {
				return err
			}
		}
		return nil
	}
}

var migrations = []migration{
	{1, "research provenance", execAll(
		`ALTER TABLE research_papers ADD COLUMN metadata_origin TEXT NOT NULL DEFAULT 'manual'`,
		`ALTER TABLE research_papers ADD COLUMN entry_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE research_papers ADD COLUMN cite_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE research_papers ADD COLUMN source_record TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE research_citations ADD COLUMN origin TEXT NOT NULL DEFAULT 'manual'`,
		`ALTER TABLE research_annotations ADD COLUMN origin TEXT NOT NULL DEFAULT 'manual'`,
	)},
	{2, "note aliases and reindex flag", execAll(
		`CREATE TABLE IF NOT EXISTS note_aliases (path TEXT NOT NULL, alias_slug TEXT NOT NULL, PRIMARY KEY(path,alias_slug))`,
		`CREATE INDEX IF NOT EXISTS idx_note_aliases_slug ON note_aliases(alias_slug)`,
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT OR REPLACE INTO meta(key,value) VALUES('reindex_required','1')`,
	)},
	{3, "math-aware full-text column", execAll(
		`ALTER TABLE notes ADD COLUMN math_text TEXT NOT NULL DEFAULT ''`,
		`DROP TRIGGER IF EXISTS notes_ai`, `DROP TRIGGER IF EXISTS notes_ad`, `DROP TRIGGER IF EXISTS notes_au`,
		`DROP TABLE IF EXISTS notes_fts`,
		`CREATE VIRTUAL TABLE notes_fts USING fts5(title, plain_text, math_text, content='notes', content_rowid='id')`,
		`CREATE TRIGGER notes_ai AFTER INSERT ON notes BEGIN
			INSERT INTO notes_fts(rowid, title, plain_text, math_text) VALUES (new.id, new.title, new.plain_text, new.math_text);
		END`,
		`CREATE TRIGGER notes_ad AFTER DELETE ON notes BEGIN
			INSERT INTO notes_fts(notes_fts, rowid, title, plain_text, math_text) VALUES('delete', old.id, old.title, old.plain_text, old.math_text);
		END`,
		`CREATE TRIGGER notes_au AFTER UPDATE ON notes BEGIN
			INSERT INTO notes_fts(notes_fts, rowid, title, plain_text, math_text) VALUES('delete', old.id, old.title, old.plain_text, old.math_text);
			INSERT INTO notes_fts(rowid, title, plain_text, math_text) VALUES (new.id, new.title, new.plain_text, new.math_text);
		END`,
		`INSERT INTO notes_fts(notes_fts) VALUES('rebuild')`,
		`INSERT OR REPLACE INTO meta(key,value) VALUES('reindex_required','1')`,
	)},
}

// SchemaVersion is the highest known migration version.
func SchemaVersion() int { return migrations[len(migrations)-1].version }

func (d *Database) hasUserData(ctx context.Context) bool {
	for _, q := range []string{`SELECT count(*) FROM notes`, `SELECT count(*) FROM research_papers`} {
		var n int
		if d.Writer.QueryRowContext(ctx, q).Scan(&n) == nil && n > 0 {
			return true
		}
	}
	return false
}

// migrate applies pending versioned migrations, each in its own transaction.
// Before the first pending migration on a database that holds data, it writes a
// consistent copy next to the database so a failed or unwanted migration can be undone.
func (d *Database) migrate(ctx context.Context) error {
	if _, err := d.Writer.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	applied := map[int]bool{}
	rows, err := d.Writer.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	var pending []migration
	for _, m := range migrations {
		if !applied[m.version] {
			pending = append(pending, m)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	if d.Path != "" && d.hasUserData(ctx) {
		if err := d.backup(ctx, pending[0].version); err != nil {
			return fmt.Errorf("pre-migration backup: %w", err)
		}
	}
	for _, m := range pending {
		tx, err := d.Writer.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := m.apply(ctx, tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,applied_at) VALUES(?,?,?)`, m.version, m.name, time.Now().UTC().Format(time.RFC3339)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// duplicateColumn tolerates databases that already received a column from an
// earlier unversioned schema pass.
func duplicateColumn(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "duplicate column")
}

func (d *Database) backup(ctx context.Context, version int) error {
	target := fmt.Sprintf("%s.pre-v%d-%s.bak", d.Path, version, time.Now().UTC().Format("20060102T150405"))
	if _, err := d.Writer.ExecContext(ctx, `VACUUM INTO ?`, target); err != nil {
		return err
	}
	matches, _ := filepath.Glob(d.Path + ".pre-v*.bak")
	sort.Strings(matches)
	for len(matches) > 3 {
		_ = os.Remove(matches[0])
		matches = matches[1:]
	}
	return nil
}

func (d *Database) ReindexRequired(ctx context.Context) bool {
	var v string
	return d.Writer.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='reindex_required'`).Scan(&v) == nil && v == "1"
}

func (d *Database) ClearReindexRequired(ctx context.Context) error {
	_, err := d.Writer.ExecContext(ctx, `DELETE FROM meta WHERE key='reindex_required'`)
	return err
}
