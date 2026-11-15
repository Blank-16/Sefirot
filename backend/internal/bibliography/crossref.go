package bibliography

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const DefaultCrossrefBase = "https://api.crossref.org"

var doiPattern = regexp.MustCompile(`^10\.\d{4,9}/\S+$`)
var tagPattern = regexp.MustCompile(`<[^>]*>`)

// ValidDOI reports whether s looks like a DOI after normalisation.
func ValidDOI(s string) bool { return doiPattern.MatchString(NormalizeDOI(s)) }

type Crossref struct {
	BaseURL string
	Client  *http.Client
}

func (c Crossref) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Lookup fetches metadata for a DOI from Crossref. The only data sent is the DOI.
// The returned Entry holds the Crossref JSON verbatim as its Raw record.
func (c Crossref) Lookup(ctx context.Context, doi string) (Entry, error) {
	doi = NormalizeDOI(doi)
	if !doiPattern.MatchString(doi) {
		return Entry{}, errors.New("not a valid DOI")
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = DefaultCrossrefBase
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/works/"+escapeDOI(doi), nil)
	if err != nil {
		return Entry{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "SEFIROT-local-archive (DOI lookup requested by the user)")
	resp, err := c.client().Do(req)
	if err != nil {
		return Entry{}, fmt.Errorf("crossref request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Entry{}, errors.New("Crossref has no record for this DOI")
	}
	if resp.StatusCode != http.StatusOK {
		return Entry{}, fmt.Errorf("crossref returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil {
		return Entry{}, err
	}
	if len(body) > 1<<20 {
		return Entry{}, errors.New("crossref response too large")
	}
	var parsed struct {
		Message struct {
			Title          []string `json:"title"`
			ContainerTitle []string `json:"container-title"`
			Author         []struct {
				Given  string `json:"given"`
				Family string `json:"family"`
				Name   string `json:"name"`
			} `json:"author"`
			Issued struct {
				DateParts [][]int `json:"date-parts"`
			} `json:"issued"`
			Abstract string   `json:"abstract"`
			URL      string   `json:"URL"`
			DOI      string   `json:"DOI"`
			Subject  []string `json:"subject"`
			Type     string   `json:"type"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Entry{}, fmt.Errorf("unreadable crossref response: %w", err)
	}
	m := parsed.Message
	if len(m.Title) == 0 || strings.TrimSpace(m.Title[0]) == "" {
		return Entry{}, errors.New("crossref record has no title")
	}
	e := Entry{Format: "crossref", Type: m.Type, Key: doi, Fields: map[string][]string{}, Raw: strings.TrimSpace(string(body))}
	e.Fields["title"] = []string{m.Title[0]}
	for _, a := range m.Author {
		name := strings.TrimSpace(a.Given + " " + a.Family)
		if name == "" {
			name = strings.TrimSpace(a.Name)
		}
		if name != "" {
			e.Fields["author"] = append(e.Fields["author"], name)
		}
	}
	if len(m.ContainerTitle) > 0 {
		e.Fields["journal"] = []string{m.ContainerTitle[0]}
	}
	if len(m.Issued.DateParts) > 0 && len(m.Issued.DateParts[0]) > 0 {
		e.Fields["year"] = []string{fmt.Sprint(m.Issued.DateParts[0][0])}
	}
	e.Fields["doi"] = []string{firstNonEmpty(m.DOI, doi)}
	e.Fields["url"] = []string{m.URL}
	e.Fields["abstract"] = []string{strings.Join(strings.Fields(tagPattern.ReplaceAllString(m.Abstract, " ")), " ")}
	e.Fields["keywords"] = m.Subject
	return e, nil
}

func escapeDOI(doi string) string {
	parts := strings.Split(doi, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
