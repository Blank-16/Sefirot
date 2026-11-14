package database

import (
	"context"
	"path"
	"sort"
	"strings"
)

// LinkIndex resolves the target of a wiki-link. A target is tried against, in
// order: the full path, the file name, the note title, and declared aliases.
// The first tier with any match decides; several matches in that tier make the
// link ambiguous and nothing is guessed.
type LinkIndex struct {
	tiers [4]map[string][]string
}

const (
	tierPath = iota
	tierBase
	tierTitle
	tierAlias
)

func (d *Database) LinkIndex(ctx context.Context) (*LinkIndex, error) {
	li := &LinkIndex{}
	for i := range li.tiers {
		li.tiers[i] = map[string][]string{}
	}
	rows, err := d.Reader.QueryContext(ctx, `SELECT path,title FROM notes ORDER BY path`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p, title string
		if err := rows.Scan(&p, &title); err != nil {
			rows.Close()
			return nil, err
		}
		stem := strings.TrimSuffix(p, path.Ext(p))
		li.add(tierPath, normalizeSlug(stem), p)
		li.add(tierBase, normalizeSlug(path.Base(stem)), p)
		if t := strings.TrimSpace(title); t != "" {
			li.add(tierTitle, strings.ToLower(t), p)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	arows, err := d.Reader.QueryContext(ctx, `SELECT path,alias_slug FROM note_aliases ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	for arows.Next() {
		var p, slug string
		if err := arows.Scan(&p, &slug); err != nil {
			return nil, err
		}
		li.add(tierAlias, slug, p)
	}
	return li, arows.Err()
}

func (li *LinkIndex) add(tier int, key, notePath string) {
	for _, existing := range li.tiers[tier][key] {
		if existing == notePath {
			return
		}
	}
	li.tiers[tier][key] = append(li.tiers[tier][key], notePath)
}

// Resolve returns the target note, or the sorted candidates when ambiguous.
// With no match both results are empty.
func (li *LinkIndex) Resolve(target string) (string, []string) {
	slug := normalizeSlug(target)
	lower := strings.ToLower(strings.TrimSpace(target))
	for tier := range li.tiers {
		key := slug
		if tier == tierTitle {
			key = lower
		}
		if found := li.tiers[tier][key]; len(found) == 1 {
			return found[0], nil
		} else if len(found) > 1 {
			c := append([]string(nil), found...)
			sort.Strings(c)
			return "", c
		}
	}
	return "", nil
}
