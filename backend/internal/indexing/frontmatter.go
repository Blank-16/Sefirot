package indexing

import (
	"strings"
)

// FrontMatter is the subset of YAML front matter SEFIROT understands: flat keys
// whose values are scalars, inline lists ([a, b]) or block lists (- a).
type FrontMatter map[string][]string

// ParseFrontMatter splits a leading "---" block from a Markdown source.
// When there is no well-formed block the source is returned unchanged.
func ParseFrontMatter(source string) (FrontMatter, string) {
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return nil, source
	}
	rest := normalized[4:]
	end := -1
	offset := 0
	for _, line := range strings.SplitAfter(rest, "\n") {
		if strings.TrimRight(line, "\n") == "---" {
			end = offset
			offset += len(line)
			break
		}
		offset += len(line)
	}
	if end < 0 {
		return nil, source
	}
	block, body := rest[:end], rest[offset:]
	meta := FrontMatter{}
	var key string
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && key != "" {
			meta[key] = append(meta[key], unquote(strings.TrimPrefix(trimmed, "- ")))
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			return nil, source
		}
		key = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		switch {
		case v == "":
			meta[key] = nil
		case strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]"):
			for _, item := range strings.Split(v[1:len(v)-1], ",") {
				if item = unquote(strings.TrimSpace(item)); item != "" {
					meta[key] = append(meta[key], item)
				}
			}
		default:
			meta[key] = []string{unquote(v)}
		}
	}
	return meta, body
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

func (f FrontMatter) first(key string) string {
	if v := f[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func (f FrontMatter) list(key string) []string {
	var out []string
	for _, v := range f[key] {
		for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ';' }) {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}
