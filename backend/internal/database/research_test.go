package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestResearchPaperRoundTripAndCascade(t *testing.T) {
	db, err := OpenDatabase(filepath.Join(t.TempDir(), "research.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	paper, err := db.SaveResearchPaper(ctx, ResearchPaper{Title: "A test of structured scientific metadata", Authors: []string{"Ada Lovelace", "Alan Turing"}, DOI: "10.1000/example", Keywords: []string{"math", "physics"}})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := db.ResearchPaper(ctx, paper.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != paper.Title || len(loaded.Authors) != 2 || len(loaded.Keywords) != 2 {
		t.Fatalf("unexpected round trip: %#v", loaded)
	}
	citation, err := db.AddResearchCitation(ctx, ResearchCitation{PaperID: paper.ID, CitationText: "Lovelace, A. (1843). Notes."})
	if err != nil {
		t.Fatal(err)
	}
	if citation.ID == 0 {
		t.Fatal("citation id was not assigned")
	}
	annotation, err := db.AddResearchAnnotation(ctx, ResearchAnnotation{PaperID: paper.ID, Page: 4, Quote: "Equation 2", Note: "Check derivation"})
	if err != nil {
		t.Fatal(err)
	}
	if annotation.ID == 0 {
		t.Fatal("annotation id was not assigned")
	}
	if err := db.DeleteResearchPaper(ctx, paper.ID); err != nil {
		t.Fatal(err)
	}
	citations, err := db.ResearchCitations(ctx, paper.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(citations) != 0 {
		t.Fatalf("expected cascade delete, got %d citations", len(citations))
	}
	annotations, err := db.ResearchAnnotations(ctx, paper.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations) != 0 {
		t.Fatalf("expected cascade delete, got %d annotations", len(annotations))
	}
}

func TestResearchRejectsEmptyTitleAndInvalidStatus(t *testing.T) {
	db, err := OpenDatabase(filepath.Join(t.TempDir(), "research.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.SaveResearchPaper(context.Background(), ResearchPaper{}); err == nil {
		t.Fatal("expected empty title validation")
	}
	if _, err := db.SaveResearchPaper(context.Background(), ResearchPaper{Title: "Paper", ReadingStatus: "maybe"}); err == nil {
		t.Fatal("expected status validation")
	}
}
