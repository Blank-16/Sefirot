package database

import (
	"context"
	"encoding/json"
	"time"
)

type Revision struct {
	ID          int64  `json:"id"`
	Path        string `json:"path"`
	CreatedAt   string `json:"createdAt"`
	ContentHash string `json:"contentHash"`
}

func (d *Database) Revisions(ctx context.Context, notePath string) ([]Revision, error) {
	rows, err := d.Reader.QueryContext(ctx, `SELECT id,path,created_at,content_hash FROM revisions WHERE path=? ORDER BY id DESC LIMIT 50`, notePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Revision{}
	for rows.Next() {
		var r Revision
		if err := rows.Scan(&r.ID, &r.Path, &r.CreatedAt, &r.ContentHash); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (d *Database) RevisionContent(ctx context.Context, id int64, notePath string) (string, error) {
	var content string
	err := d.Reader.QueryRowContext(ctx, `SELECT content_html FROM revisions WHERE id=? AND path=?`, id, notePath).Scan(&content)
	return content, err
}

type EmbeddingDocument struct {
	Path        string `json:"path"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	PlainText   string `json:"plainText"`
	ContentHash string `json:"contentHash"`
}
type StoredEmbedding struct {
	Path    string    `json:"path"`
	Title   string    `json:"title"`
	Summary string    `json:"summary"`
	Vector  []float64 `json:"vector"`
}

func (d *Database) PendingEmbeddings(ctx context.Context, model string, limit int) ([]EmbeddingDocument, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	rows, err := d.Reader.QueryContext(ctx, `SELECT n.path,n.title,n.summary,n.plain_text,n.content_hash FROM notes n LEFT JOIN embeddings e ON e.path=n.path AND e.model=? WHERE e.path IS NULL OR e.content_hash<>n.content_hash ORDER BY n.updated_at DESC LIMIT ?`, model, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmbeddingDocument{}
	for rows.Next() {
		var v EmbeddingDocument
		if err := rows.Scan(&v.Path, &v.Title, &v.Summary, &v.PlainText, &v.ContentHash); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (d *Database) StoreEmbedding(ctx context.Context, path, model, hash string, vector []float64) error {
	encoded, err := json.Marshal(vector)
	if err != nil {
		return err
	}
	_, err = d.Writer.ExecContext(ctx, `INSERT INTO embeddings(path,model,content_hash,vector_json,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET model=excluded.model,content_hash=excluded.content_hash,vector_json=excluded.vector_json,updated_at=excluded.updated_at`, path, model, hash, string(encoded), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (d *Database) LoadEmbeddings(ctx context.Context, model string) ([]StoredEmbedding, error) {
	rows, err := d.Reader.QueryContext(ctx, `SELECT n.path,n.title,n.summary,e.vector_json FROM embeddings e JOIN notes n ON n.path=e.path WHERE e.model=? AND e.content_hash=n.content_hash`, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StoredEmbedding{}
	for rows.Next() {
		var v StoredEmbedding
		var raw string
		if err := rows.Scan(&v.Path, &v.Title, &v.Summary, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &v.Vector); err != nil {
			continue
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type AgentRun struct {
	ID               int64  `json:"id"`
	Profile          string `json:"profile"`
	WorkingDirectory string `json:"workingDirectory"`
	StartedAt        string `json:"startedAt"`
	EndedAt          string `json:"endedAt,omitempty"`
	Status           string `json:"status"`
	ExitCode         int    `json:"exitCode"`
}

func (d *Database) StartAgentRun(ctx context.Context, profile, workingDirectory string) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := d.Writer.ExecContext(ctx, `INSERT INTO agent_runs(profile,working_directory,started_at,status) VALUES(?,?,?,?)`, profile, workingDirectory, time.Now().UTC().Format(time.RFC3339Nano), "running")
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	_, _ = d.Writer.ExecContext(ctx, `DELETE FROM agent_runs WHERE id NOT IN (SELECT id FROM agent_runs ORDER BY id DESC LIMIT 2000)`)
	return id, nil
}
func (d *Database) FinishAgentRun(ctx context.Context, id int64, status string, exitCode int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := d.Writer.ExecContext(ctx, `UPDATE agent_runs SET ended_at=?,status=?,exit_code=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339Nano), status, exitCode, id)
	return err
}
func (d *Database) AgentRuns(ctx context.Context, limit int) ([]AgentRun, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := d.Reader.QueryContext(ctx, `SELECT id,profile,working_directory,started_at,ended_at,status,exit_code FROM agent_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgentRun{}
	for rows.Next() {
		var v AgentRun
		if err := rows.Scan(&v.ID, &v.Profile, &v.WorkingDirectory, &v.StartedAt, &v.EndedAt, &v.Status, &v.ExitCode); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
