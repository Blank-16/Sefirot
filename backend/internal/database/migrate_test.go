package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigrationFromUnversionedDatabaseBacksUpAndPreservesData(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "index.db")
	db, err := OpenDatabase(file)
	if err != nil {
		t.Fatal(err)
	}
	paper, err := db.SaveResearchPaper(ctx, ResearchPaper{Title: "Legacy paper", DOI: "10.1/legacy"})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`ALTER TABLE research_papers DROP COLUMN metadata_origin`,
		`ALTER TABLE research_papers DROP COLUMN entry_type`,
		`ALTER TABLE research_papers DROP COLUMN cite_key`,
		`ALTER TABLE research_papers DROP COLUMN source_record`,
		`ALTER TABLE research_citations DROP COLUMN origin`,
		`ALTER TABLE research_annotations DROP COLUMN origin`,
		`DROP TABLE note_aliases`, `DROP TABLE meta`, `DROP TABLE schema_migrations`,
	} {
		if _, err := db.Writer.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	db.Close()

	db, err = OpenDatabase(file)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	defer db.Close()
	got, err := db.ResearchPaper(ctx, paper.ID)
	if err != nil || got.Title != "Legacy paper" || got.MetadataOrigin != "manual" {
		t.Fatalf("paper after migration: %+v %v", got, err)
	}
	backups, _ := filepath.Glob(file + ".pre-v*.bak")
	if len(backups) != 1 {
		t.Fatalf("expected one pre-migration backup, got %v", backups)
	}
	raw, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatal(err)
	}
	var kept int
	if err := raw.QueryRow(`SELECT count(*) FROM research_papers WHERE title='Legacy paper'`).Scan(&kept); err != nil || kept != 1 {
		t.Fatalf("backup does not hold the pre-migration data: %d %v", kept, err)
	}
	raw.Close()
	if !db.ReindexRequired(ctx) {
		t.Fatal("alias migration must request a reindex")
	}
	var n int
	if err := db.Writer.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil || n != len(migrations) {
		t.Fatalf("recorded migrations = %d, want %d (%v)", n, len(migrations), err)
	}
	db.Close()
	db, err = OpenDatabase(file)
	if err != nil {
		t.Fatal(err)
	}
	if backups, _ := filepath.Glob(file + ".pre-v*.bak"); len(backups) != 1 {
		t.Fatalf("an up-to-date database must not be backed up again: %v", backups)
	}
}

func TestImportedMetadataEditsAreMarkedAndSourceKept(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDatabase(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p, err := db.SaveResearchPaper(ctx, ResearchPaper{Title: "Original Title", MetadataOrigin: "bibtex", EntryType: "article", CiteKey: "orig2020", SourceRecord: "@article{orig2020, title={Original Title}}"})
	if err != nil {
		t.Fatal(err)
	}
	p.ReadingStatus = "read"
	p.SourceRecord = "tampered"
	p.MetadataOrigin = "manual"
	updated, err := db.SaveResearchPaper(ctx, p)
	if err != nil || updated.MetadataOrigin != "bibtex" {
		t.Fatalf("status-only edit must not change origin: %+v %v", updated, err)
	}
	p.Title = "Corrected Title"
	updated, err = db.SaveResearchPaper(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := db.ResearchPaper(ctx, p.ID)
	if got.MetadataOrigin != "bibtex-edited" || got.SourceRecord != "@article{orig2020, title={Original Title}}" || got.CiteKey != "orig2020" {
		t.Fatalf("provenance after edit: %+v", got)
	}
	if _, err := db.AddResearchAnnotation(ctx, ResearchAnnotation{PaperID: p.ID, Note: "x", Origin: "oracle"}); err == nil {
		t.Fatal("unknown origin must be rejected")
	}
}
