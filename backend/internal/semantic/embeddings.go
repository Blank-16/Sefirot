package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"sefirot/internal/database"
)

// EmbeddingService provides optional local semantic retrieval through Ollama.
// It never sends vault content to a remote service: only loopback endpoints are accepted.
type EmbeddingService struct {
	db     *database.Database
	base   string
	model  string
	client *http.Client
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type ollamaEmbedRequest struct {
	Model string `json:"model"`
	Input any    `json:"input"`
}
type ollamaEmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Error      string      `json:"error"`
}
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

type SemanticResult struct {
	Path       string  `json:"path"`
	Title      string  `json:"title"`
	Summary    string  `json:"summary"`
	Similarity float64 `json:"similarity"`
}

func envWithLegacy(primary, legacy string) string {
	if value := strings.TrimSpace(os.Getenv(primary)); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv(legacy))
}

func NewEmbeddingService(db *database.Database) *EmbeddingService {
	base := strings.TrimRight(envWithLegacy("SEFIROT_OLLAMA_URL", "HTML_VAULT_OLLAMA_URL"), "/")
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	// Local-only by design. Do not allow a remote embedding endpoint to receive vault data.
	u, parseErr := url.Parse(base)
	if parseErr != nil || u.Scheme != "http" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.Hostname() == "" || (u.Hostname() != "localhost" && (net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback())) {
		base = "http://127.0.0.1:11434"
	}
	model := envWithLegacy("SEFIROT_EMBED_MODEL", "HTML_VAULT_EMBED_MODEL")
	if model == "" {
		model = "nomic-embed-text"
	}
	return &EmbeddingService{db: db, base: base, model: model, client: &http.Client{Timeout: 35 * time.Second}}
}

func (s *EmbeddingService) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.indexBatch(ctx, 8); err != nil && !errors.Is(err, errOllamaUnavailable) {
			log.Printf("semantic indexing: %v", err)
		}
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.indexBatch(ctx, 8); err != nil && !errors.Is(err, errOllamaUnavailable) {
					log.Printf("semantic indexing: %v", err)
				}
			}
		}
	}()
}

func (s *EmbeddingService) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

var errOllamaUnavailable = errors.New("Ollama is unavailable or the configured embedding model is not installed")

func (s *EmbeddingService) modelAvailable(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/api/tags", nil)
	if err != nil {
		return false, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return false, errOllamaUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, errOllamaUnavailable
	}
	var tags ollamaTagsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&tags); err != nil {
		return false, err
	}
	for _, m := range tags.Models {
		if m.Name == s.model || strings.HasPrefix(m.Name, s.model+":") {
			return true, nil
		}
	}
	return false, errOllamaUnavailable
}

func (s *EmbeddingService) embed(ctx context.Context, input string) ([]float64, error) {
	vectors, err := s.embedBatch(ctx, []string{input})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, errors.New("Ollama returned an empty embedding")
	}
	return vectors[0], nil
}
func (s *EmbeddingService) embedBatch(ctx context.Context, inputs []string) ([][]float64, error) {
	body, _ := json.Marshal(ollamaEmbedRequest{Model: s.model, Input: inputs})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, errOllamaUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Ollama embedding request failed (%s): %s", resp.Status, strings.TrimSpace(string(payload)))
	}
	var result ollamaEmbedResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&result); err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, errors.New(result.Error)
	}
	if len(result.Embeddings) != len(inputs) {
		return nil, fmt.Errorf("Ollama returned %d embeddings for %d inputs", len(result.Embeddings), len(inputs))
	}
	for _, v := range result.Embeddings {
		if len(v) == 0 {
			return nil, errors.New("Ollama returned an empty embedding")
		}
	}
	return result.Embeddings, nil
}

func (s *EmbeddingService) indexBatch(ctx context.Context, limit int) error {
	available, err := s.modelAvailable(ctx)
	if err != nil || !available {
		return errOllamaUnavailable
	}
	docs, err := s.db.PendingEmbeddings(ctx, s.model, limit)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return nil
	}
	inputs := make([]string, len(docs))
	for i, doc := range docs {
		text := doc.Title + "\n" + doc.Summary + "\n" + doc.PlainText
		if len(text) > 12000 {
			text = text[:12000]
		}
		inputs[i] = text
	}
	vectors, err := s.embedBatch(ctx, inputs)
	if err != nil {
		return err
	}
	for i, doc := range docs {
		if err := s.db.StoreEmbedding(ctx, doc.Path, s.model, doc.ContentHash, vectors[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *EmbeddingService) Status(ctx context.Context) map[string]any {
	available, err := s.modelAvailable(ctx)
	var indexed, pending int
	_ = s.db.Reader.QueryRowContext(ctx, `SELECT COUNT(*) FROM embeddings WHERE model=?`, s.model).Scan(&indexed)
	_ = s.db.Reader.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes n LEFT JOIN embeddings e ON e.path=n.path AND e.model=? AND e.content_hash=n.content_hash WHERE e.path IS NULL`, s.model).Scan(&pending)
	return map[string]any{"enabled": available, "provider": "ollama-local", "model": s.model, "indexed": indexed, "pending": pending, "error": errorString(err)}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *EmbeddingService) Search(ctx context.Context, query string, limit int) ([]SemanticResult, error) {
	available, err := s.modelAvailable(ctx)
	if err != nil || !available {
		return nil, errOllamaUnavailable
	}
	queryVector, err := s.embed(ctx, query)
	if err != nil {
		return nil, err
	}
	items, err := s.db.LoadEmbeddings(ctx, s.model)
	if err != nil {
		return nil, err
	}
	results := make([]SemanticResult, 0, len(items))
	for _, item := range items {
		similarity := cosineSimilarity(queryVector, item.Vector)
		if similarity <= 0 {
			continue
		}
		results = append(results, SemanticResult{Path: item.Path, Title: item.Title, Summary: item.Summary, Similarity: similarity})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Similarity > results[j].Similarity })
	if limit < 1 || limit > 50 {
		limit = 20
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, aa, bb float64
	for i := range a {
		dot += a[i] * b[i]
		aa += a[i] * a[i]
		bb += b[i] * b[i]
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / (math.Sqrt(aa) * math.Sqrt(bb))
}

func (s *EmbeddingService) Model() string { return s.model }
