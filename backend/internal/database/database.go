package database

import (
	"context"
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

type DocumentHeading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
}
type DocumentRelationship struct {
	Target string `json:"target"`
	Type   string `json:"type"`
}
type IndexedNote struct {
	Path          string
	Title         string
	PlainText     string
	Summary       string
	SummaryShort  string
	SummaryDetail string
	Headings      []DocumentHeading
	Description   string
	DocType       string
	Tags          []string
	Aliases       []string
	ContentHash   string
	ContentHTML   string
	Links         []string
	Relationships []DocumentRelationship
	UpdatedAt     string
}

type Database struct {
	Reader *sql.DB
	Writer *sql.DB
	Path   string
}

type GraphNode struct {
	ID           string   `json:"id"`
	Path         string   `json:"path"`
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	SummaryShort string   `json:"summaryShort"`
	Description  string   `json:"description"`
	DocType      string   `json:"type"`
	Tags         []string `json:"tags"`
	Degree       int      `json:"degree"`
}
type GraphEdge struct {
	Source string   `json:"source"`
	Target string   `json:"target"`
	Type   string   `json:"type"`
	Types  []string `json:"types"`
}
type ContextNote struct {
	Path          string            `json:"path"`
	Title         string            `json:"title"`
	Summary       string            `json:"summary"`
	SummaryShort  string            `json:"summaryShort,omitempty"`
	SummaryDetail string            `json:"summaryDetail,omitempty"`
	Headings      []DocumentHeading `json:"headings,omitempty"`
	Description   string            `json:"description,omitempty"`
	DocType       string            `json:"type,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
}

type SearchResult struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Updated string `json:"updatedAt"`
}

type Backlink struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

func OpenDatabase(filename string) (*Database, error) {
	abs, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "windows" {
		abs = "/" + filepath.ToSlash(abs)
	}
	u := url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "foreign_keys(ON)")
	u.RawQuery = q.Encode()
	dsn := u.String()

	writer, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	reader, err := sql.Open("sqlite", dsn)
	if err != nil {
		writer.Close()
		return nil, err
	}
	reader.SetMaxOpenConns(6)
	reader.SetMaxIdleConns(3)

	if err := writer.Ping(); err != nil {
		writer.Close()
		reader.Close()
		return nil, err
	}
	if err := reader.Ping(); err != nil {
		writer.Close()
		reader.Close()
		return nil, err
	}
	db := &Database{Reader: reader, Writer: writer, Path: filename}
	if err := db.schema(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := db.ResearchSchema(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := db.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (d *Database) schema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT UNIQUE NOT NULL,
			title TEXT NOT NULL,
			plain_text TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`ALTER TABLE notes ADD COLUMN summary TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notes ADD COLUMN summary_short TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notes ADD COLUMN summary_detail TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notes ADD COLUMN headings_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE notes ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notes ADD COLUMN doc_type TEXT NOT NULL DEFAULT 'note'`,
		`ALTER TABLE notes ADD COLUMN tags_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE notes ADD COLUMN content_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notes ADD COLUMN content_html TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS note_tags (path TEXT NOT NULL, tag TEXT NOT NULL, PRIMARY KEY(path,tag))`,
		`CREATE INDEX IF NOT EXISTS idx_note_tags_tag ON note_tags(tag)`,
		`CREATE TABLE IF NOT EXISTS relationships (source_path TEXT NOT NULL, target_slug TEXT NOT NULL, relation_type TEXT NOT NULL, PRIMARY KEY(source_path,target_slug,relation_type))`,
		`CREATE INDEX IF NOT EXISTS idx_relationships_target ON relationships(target_slug)`,
		`CREATE TABLE IF NOT EXISTS revisions (id INTEGER PRIMARY KEY AUTOINCREMENT, path TEXT NOT NULL, content_hash TEXT NOT NULL, content_html TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS embeddings (path TEXT PRIMARY KEY, model TEXT NOT NULL, content_hash TEXT NOT NULL, vector_json TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS agent_runs (id INTEGER PRIMARY KEY AUTOINCREMENT, profile TEXT NOT NULL, working_directory TEXT NOT NULL, started_at TEXT NOT NULL, ended_at TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, exit_code INTEGER NOT NULL DEFAULT -1)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_runs_started ON agent_runs(started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_embeddings_model ON embeddings(model)`,
		`CREATE INDEX IF NOT EXISTS idx_revisions_path_created ON revisions(path,created_at DESC)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(title, plain_text, content='notes', content_rowid='id')`,
		`CREATE TRIGGER IF NOT EXISTS notes_ai AFTER INSERT ON notes BEGIN
			INSERT INTO notes_fts(rowid, title, plain_text) VALUES (new.id, new.title, new.plain_text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS notes_ad AFTER DELETE ON notes BEGIN
			INSERT INTO notes_fts(notes_fts, rowid, title, plain_text) VALUES('delete', old.id, old.title, old.plain_text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS notes_au AFTER UPDATE ON notes BEGIN
			INSERT INTO notes_fts(notes_fts, rowid, title, plain_text) VALUES('delete', old.id, old.title, old.plain_text);
			INSERT INTO notes_fts(rowid, title, plain_text) VALUES (new.id, new.title, new.plain_text);
		END`,
		`CREATE TABLE IF NOT EXISTS links (
			source_path TEXT NOT NULL,
			target_slug TEXT NOT NULL,
			PRIMARY KEY(source_path, target_slug)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_links_target ON links(target_slug)`,
		`CREATE INDEX IF NOT EXISTS idx_notes_path ON notes(path)`,
		`CREATE INDEX IF NOT EXISTS idx_notes_doc_type ON notes(doc_type)`,
		`CREATE INDEX IF NOT EXISTS idx_notes_updated_at ON notes(updated_at DESC)`,
	}
	for _, s := range statements {
		if _, err := d.Writer.ExecContext(ctx, s); err != nil {
			if strings.HasPrefix(s, "ALTER TABLE") && strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				continue
			}
			return fmt.Errorf("schema: %w", err)
		}
	}
	return nil
}
