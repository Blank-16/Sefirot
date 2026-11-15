// Package bibliography parses and formats reference data (BibTeX, RIS and Crossref
// records). Parsers never discard the original text: every Entry carries the
// verbatim source record so imported metadata stays traceable and recoverable.
package bibliography

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

type Entry struct {
	Format string // "bibtex" or "ris"
	Type   string
	Key    string
	Fields map[string][]string
	Raw    string
}

func (e Entry) Field(name string) string {
	if v := e.Fields[name]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// ParseBibTeX reads every entry in text. Malformed entries are reported in the
// returned error list and skipped; the rest are still returned.
func ParseBibTeX(text string) ([]Entry, []error) {
	var entries []Entry
	var errs []error
	macros := map[string]string{}
	i := 0
	for i < len(text) {
		at := strings.IndexByte(text[i:], '@')
		if at < 0 {
			break
		}
		start := i + at
		p := start + 1
		for p < len(text) && (unicode.IsLetter(rune(text[p])) || text[p] == '_') {
			p++
		}
		kind := strings.ToLower(text[start+1 : p])
		for p < len(text) && unicode.IsSpace(rune(text[p])) {
			p++
		}
		if kind == "" || p >= len(text) || (text[p] != '{' && text[p] != '(') {
			i = start + 1
			continue
		}
		closer := byte('}')
		if text[p] == '(' {
			closer = ')'
		}
		end, err := matchDelimiter(text, p, text[p], closer)
		if err != nil {
			errs = append(errs, fmt.Errorf("entry at byte %d: %w", start, err))
			i = start + 1
			continue
		}
		body := text[p+1 : end]
		raw := text[start : end+1]
		i = end + 1
		switch kind {
		case "comment", "preamble":
			continue
		case "string":
			if name, val, ok := strings.Cut(body, "="); ok {
				if v, err := parseValue(strings.TrimSpace(val), macros); err == nil {
					macros[strings.ToLower(strings.TrimSpace(name))] = v
				}
			}
			continue
		}
		key, rest, ok := strings.Cut(body, ",")
		if !ok {
			key, rest = body, ""
		}
		entry := Entry{Format: "bibtex", Type: kind, Key: strings.TrimSpace(key), Fields: map[string][]string{}, Raw: raw}
		if err := parseFields(rest, macros, &entry); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.Key, err))
			continue
		}
		entries = append(entries, entry)
	}
	return entries, errs
}

func matchDelimiter(text string, open int, o, c byte) (int, error) {
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '\\':
			i++
		case o:
			depth++
		case c:
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, errors.New("unbalanced delimiters")
}

func parseFields(s string, macros map[string]string, e *Entry) error {
	i := 0
	for i < len(s) {
		for i < len(s) && (unicode.IsSpace(rune(s[i])) || s[i] == ',') {
			i++
		}
		if i >= len(s) {
			break
		}
		eq := strings.IndexByte(s[i:], '=')
		if eq < 0 {
			if strings.TrimSpace(s[i:]) == "" {
				break
			}
			return fmt.Errorf("field without value near %q", truncate(s[i:], 30))
		}
		name := strings.ToLower(strings.TrimSpace(s[i : i+eq]))
		i += eq + 1
		for i < len(s) && unicode.IsSpace(rune(s[i])) {
			i++
		}
		valEnd, err := valueEnd(s, i)
		if err != nil {
			return fmt.Errorf("field %s: %w", name, err)
		}
		value, err := parseValue(strings.TrimSpace(s[i:valEnd]), macros)
		if err != nil {
			return fmt.Errorf("field %s: %w", name, err)
		}
		e.Fields[name] = append(e.Fields[name], value)
		i = valEnd
	}
	return nil
}

func valueEnd(s string, i int) (int, error) {
	depth := 0
	inQuote := false
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '{':
			if !inQuote {
				depth++
			}
		case '}':
			if !inQuote {
				depth--
				if depth < 0 {
					return 0, errors.New("unbalanced braces")
				}
			}
		case '"':
			if depth == 0 {
				inQuote = !inQuote
			}
		case ',':
			if depth == 0 && !inQuote {
				return j, nil
			}
		}
	}
	if depth != 0 || inQuote {
		return 0, errors.New("unterminated value")
	}
	return len(s), nil
}

// parseValue evaluates a BibTeX value: braced or quoted strings, numbers, macro
// names and "#" concatenations. Braces inside values are kept; display code strips them.
func parseValue(v string, macros map[string]string) (string, error) {
	var out strings.Builder
	for _, part := range splitConcat(v) {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case part[0] == '{' && part[len(part)-1] == '}':
			out.WriteString(part[1 : len(part)-1])
		case part[0] == '"' && len(part) >= 2 && part[len(part)-1] == '"':
			out.WriteString(part[1 : len(part)-1])
		default:
			if m, ok := macros[strings.ToLower(part)]; ok {
				out.WriteString(m)
			} else if builtinMonth(part) != "" {
				out.WriteString(builtinMonth(part))
			} else {
				out.WriteString(part)
			}
		}
	}
	return strings.Join(strings.Fields(out.String()), " "), nil
}

func splitConcat(v string) []string {
	var parts []string
	depth, last := 0, 0
	inQuote := false
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
		case '"':
			if depth == 0 {
				inQuote = !inQuote
			}
		case '#':
			if depth == 0 && !inQuote {
				parts = append(parts, v[last:i])
				last = i + 1
			}
		}
	}
	return append(parts, v[last:])
}

func builtinMonth(s string) string {
	months := map[string]string{"jan": "January", "feb": "February", "mar": "March", "apr": "April", "may": "May", "jun": "June", "jul": "July", "aug": "August", "sep": "September", "oct": "October", "nov": "November", "dec": "December"}
	return months[strings.ToLower(s)]
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// Plain removes BibTeX protective braces for display without touching the stored source.
func Plain(s string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("{", "", "}", "", `\&`, "&", `\_`, "_", `\%`, "%").Replace(s)), " ")
}
