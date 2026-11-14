package database

import "context"

type BrokenLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type AmbiguousLink struct {
	Source     string   `json:"source"`
	Target     string   `json:"target"`
	Candidates []string `json:"candidates"`
}

type IntegrityReport struct {
	Notes     int             `json:"notes"`
	Broken    []BrokenLink    `json:"brokenLinks"`
	Ambiguous []AmbiguousLink `json:"ambiguousLinks"`
	Orphans   []string        `json:"orphans"`
}

// Integrity reports links that resolve to no note, links whose target matches
// several notes (and so cannot be resolved safely), and notes with no links in or out.
func (d *Database) Integrity(ctx context.Context) (IntegrityReport, error) {
	rep := IntegrityReport{Broken: []BrokenLink{}, Ambiguous: []AmbiguousLink{}, Orphans: []string{}}
	idx, err := d.LinkIndex(ctx)
	if err != nil {
		return rep, err
	}
	rows, err := d.Reader.QueryContext(ctx, `SELECT path FROM notes ORDER BY path`)
	if err != nil {
		return rep, err
	}
	var notes []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return rep, err
		}
		notes = append(notes, p)
	}
	rows.Close()
	rep.Notes = len(notes)
	lrows, err := d.Reader.QueryContext(ctx, `SELECT source_path,target_slug FROM links ORDER BY source_path,target_slug`)
	if err != nil {
		return rep, err
	}
	defer lrows.Close()
	linked := map[string]bool{}
	for lrows.Next() {
		var src, slug string
		if err := lrows.Scan(&src, &slug); err != nil {
			return rep, err
		}
		target, candidates := idx.Resolve(slug)
		switch {
		case target != "":
			if target != src {
				linked[src], linked[target] = true, true
			}
		case len(candidates) > 0:
			rep.Ambiguous = append(rep.Ambiguous, AmbiguousLink{Source: src, Target: slug, Candidates: candidates})
			linked[src] = true
			for _, c := range candidates {
				linked[c] = true
			}
		default:
			rep.Broken = append(rep.Broken, BrokenLink{Source: src, Target: slug})
		}
	}
	if err := lrows.Err(); err != nil {
		return rep, err
	}
	for _, p := range notes {
		if !linked[p] {
			rep.Orphans = append(rep.Orphans, p)
		}
	}
	return rep, nil
}
