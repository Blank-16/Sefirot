package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDatabaseIndexSearchGraphAndRevisions(t *testing.T) {
	db, err := OpenDatabase(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	first := IndexedNote{Path: "Engineering/Systems.html", Title: "Distributed Systems", PlainText: "Replication consensus and fault tolerance", Summary: "Distributed systems concepts", DocType: "concept", Tags: []string{"systems", "architecture"}, ContentHash: "hash-1", ContentHTML: "<h1>v1</h1>", Links: []string{"Consensus"}, Relationships: []DocumentRelationship{{Target: "Consensus", Type: "depends-on"}, {Target: "Consensus", Type: "references"}}}
	if err := db.Upsert(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ContentHash = "hash-2"
	second.ContentHTML = "<h1>v2</h1>"
	second.PlainText += " and replication strategies"
	if err := db.Upsert(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := db.Upsert(ctx, IndexedNote{Path: "Consensus.html", Title: "Consensus", PlainText: "Agreement among distributed nodes", Summary: "Consensus protocols", ContentHash: "consensus-1", ContentHTML: "<h1>Consensus</h1>"}); err != nil {
		t.Fatal(err)
	}
	results, err := db.Search(ctx, "replication")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != first.Path {
		t.Fatalf("search results = %#v", results)
	}
	filtered, err := db.SearchFiltered(ctx, "", "systems", "concept", "Engineering")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filtered results = %#v", filtered)
	}
	runID, err := db.StartAgentRun(ctx, "codex", "/vault")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinishAgentRun(ctx, runID, "completed", 0); err != nil {
		t.Fatal(err)
	}
	runs, err := db.AgentRuns(ctx, 10)
	if err != nil || len(runs) != 1 || runs[0].Status != "completed" {
		t.Fatalf("agent runs: %#v, err=%v", runs, err)
	}
	revisions, err := db.Revisions(ctx, first.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 {
		t.Fatalf("revision count = %d", len(revisions))
	}
	content, err := db.RevisionContent(ctx, revisions[0].ID, first.Path)
	if err != nil {
		t.Fatal(err)
	}
	if content != "<h1>v1</h1>" {
		t.Fatalf("revision content = %q", content)
	}
	nodes, edges, err := db.Graph(ctx, first.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || len(edges) != 1 || len(edges[0].Types) != 2 || nodes[0].SummaryShort == "" {
		t.Fatalf("graph nodes=%d edges=%#v", len(nodes), edges)
	}
}
