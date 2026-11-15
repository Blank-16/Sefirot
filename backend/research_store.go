package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"

	"sefirot/internal/database"
	"sefirot/internal/indexing"
	"sefirot/internal/vaultfs"
)

const researchRecordName = "record.json"

func researchRecordPath(id int64) string {
	return path.Join("research", strconv.FormatInt(id, 10), researchRecordName)
}

// persistResearch writes the authoritative copy of a research record into the vault.
// The SQLite rows are an index of these files and can be rebuilt from them.
func (a *App) persistResearch(ctx context.Context, id int64) error {
	rec, err := a.db.ResearchRecord(ctx, id)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err := vaultfs.WriteAtomic(a.root, researchRecordPath(id), raw); err != nil {
		return fmt.Errorf("saved to index but vault record not written: %w", err)
	}
	return nil
}

// rebuildResearch restores records from the vault into the index.
func (a *App) rebuildResearch(ctx context.Context) (int, error) {
	dirs, err := indexing.ReadDir(a.root, "research")
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	restored := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		rel := path.Join("research", d.Name(), researchRecordName)
		raw, err := indexing.ReadRootFile(a.root, rel)
		if err != nil {
			continue
		}
		var rec database.ResearchRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			log.Printf("research record %s: %v", rel, err)
			continue
		}
		if id, perr := strconv.ParseInt(d.Name(), 10, 64); perr != nil || id != rec.Paper.ID {
			log.Printf("research record %s: id does not match directory", rel)
			continue
		}
		changed, err := a.db.RestoreResearchRecord(ctx, rec)
		if err != nil {
			log.Printf("research record %s: %v", rel, err)
			continue
		}
		if changed {
			restored++
		}
	}
	return restored, nil
}
