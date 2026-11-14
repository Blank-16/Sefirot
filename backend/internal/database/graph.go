package database

import (
	"context"
	"database/sql"
	"encoding/json"
)

func (d *Database) Graph(ctx context.Context, focus string) ([]GraphNode, []GraphEdge, error) {
	rows, err := d.Reader.QueryContext(ctx, `SELECT path,title,summary,summary_short,description,doc_type,tags_json FROM notes ORDER BY path`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	nodes := []GraphNode{}
	for rows.Next() {
		var n GraphNode
		var tags string
		if err := rows.Scan(&n.Path, &n.Title, &n.Summary, &n.SummaryShort, &n.Description, &n.DocType, &tags); err != nil {
			return nil, nil, err
		}
		_ = json.Unmarshal([]byte(tags), &n.Tags)
		n.ID = n.Path
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	linkIdx, err := d.LinkIndex(ctx)
	if err != nil {
		return nil, nil, err
	}
	erows, err := d.Reader.QueryContext(ctx, `SELECT source_path,target_slug,relation_type FROM relationships ORDER BY source_path,target_slug,relation_type`)
	if err != nil {
		return nil, nil, err
	}
	defer erows.Close()
	edgeIndex := map[string]int{}
	degrees := map[string]int{}
	edges := []GraphEdge{}
	for erows.Next() {
		var src, slug, relType string
		if err := erows.Scan(&src, &slug, &relType); err != nil {
			return nil, nil, err
		}
		target, _ := linkIdx.Resolve(slug)
		if target == "" || target == src {
			continue
		}
		key := src + "\x00" + target
		if idx, ok := edgeIndex[key]; ok {
			edges[idx].Types = append(edges[idx].Types, relType)
			continue
		}
		edgeIndex[key] = len(edges)
		edges = append(edges, GraphEdge{Source: src, Target: target, Type: relType, Types: []string{relType}})
		degrees[src]++
		degrees[target]++
	}
	if err := erows.Err(); err != nil {
		return nil, nil, err
	}
	for i := range nodes {
		nodes[i].Degree = degrees[nodes[i].ID]
	}
	return nodes, edges, erows.Err()
}

func (d *Database) AIContext(ctx context.Context, notePath string) (map[string]any, error) {
	var current ContextNote
	var tags, headings string
	err := d.Reader.QueryRowContext(ctx, `SELECT path,title,summary,summary_short,summary_detail,headings_json,description,doc_type,tags_json FROM notes WHERE path=?`, notePath).Scan(&current.Path, &current.Title, &current.Summary, &current.SummaryShort, &current.SummaryDetail, &headings, &current.Description, &current.DocType, &tags)
	_ = json.Unmarshal([]byte(tags), &current.Tags)
	_ = json.Unmarshal([]byte(headings), &current.Headings)
	if err != nil {
		return nil, err
	}
	_, edges, err := d.Graph(ctx, notePath)
	if err != nil {
		return nil, err
	}
	byID := map[string]ContextNote{notePath: current}
	needed := map[string]struct{}{}
	for _, edge := range edges {
		if edge.Source == notePath {
			needed[edge.Target] = struct{}{}
		}
		if edge.Target == notePath {
			needed[edge.Source] = struct{}{}
		}
	}
	for p := range needed {
		var item ContextNote
		var tagJSON, headingJSON string
		err := d.Reader.QueryRowContext(ctx, `SELECT path,title,summary,summary_short,summary_detail,headings_json,description,doc_type,tags_json FROM notes WHERE path=?`, p).Scan(&item.Path, &item.Title, &item.Summary, &item.SummaryShort, &item.SummaryDetail, &headingJSON, &item.Description, &item.DocType, &tagJSON)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagJSON), &item.Tags)
		_ = json.Unmarshal([]byte(headingJSON), &item.Headings)
		byID[p] = item
	}
	incoming := []ContextNote{}
	outgoing := []ContextNote{}
	currentRelationships := []GraphEdge{}
	for _, e := range edges {
		if e.Target == notePath {
			if n, ok := byID[e.Source]; ok {
				incoming = append(incoming, n)
			}
		}
		if e.Source == notePath {
			currentRelationships = append(currentRelationships, e)
			if n, ok := byID[e.Target]; ok {
				outgoing = append(outgoing, n)
			}
		}
	}
	truncated := false
	const contextLimit = 30
	if len(incoming) > contextLimit {
		incoming = incoming[:contextLimit]
		truncated = true
	}
	if len(outgoing) > contextLimit {
		outgoing = outgoing[:contextLimit]
		truncated = true
	}
	if len(currentRelationships) > contextLimit {
		currentRelationships = currentRelationships[:contextLimit]
		truncated = true
	}
	return map[string]any{"protocol": "html-vault-context-v1", "current": current, "backlinks": incoming, "linkedDocuments": outgoing, "relationships": currentRelationships, "truncated": truncated, "contextLimit": contextLimit, "instruction": "Use summaries, tags, backlinks, and typed relationships to shortlist relevant documents. Read only necessary full HTML. Preserve existing wiki-links and add typed data-relation anchors for meaningful relationships. After writing files, verify links and let the filesystem watcher index them."}, nil
}
