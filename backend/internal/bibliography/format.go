package bibliography

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Reference is the subset of paper metadata needed to format citations.
type Reference struct {
	Key         string
	EntryType   string
	Title       string
	Authors     []string
	Publication string
	Year        string
	DOI         string
	URL         string
	Abstract    string
	Keywords    []string
	Raw         string // verbatim imported record, used for lossless export
	Edited      bool   // true when the record was changed after import
	Format      string // format of Raw
}

// FromEntry maps a parsed entry to a Reference. Values are display-cleaned; Raw is untouched.
func FromEntry(e Entry) Reference {
	r := Reference{Key: e.Key, EntryType: e.Type, Raw: e.Raw, Format: e.Format}
	if e.Format == "ris" {
		r.Title = firstNonEmpty(e.Field("TI"), e.Field("T1"))
		r.Authors = append(append([]string{}, e.Fields["AU"]...), e.Fields["A1"]...)
		for i, a := range r.Authors {
			r.Authors[i] = flipName(a)
		}
		r.Abstract = firstNonEmpty(e.Field("AB"), e.Field("N2"))
		r.DOI = NormalizeDOI(e.Field("DO"))
		r.Publication = firstNonEmpty(e.Field("JO"), e.Field("JF"), e.Field("T2"), e.Field("JA"))
		r.Year = yearOf(firstNonEmpty(e.Field("PY"), e.Field("Y1"), e.Field("DA")))
		r.URL = e.Field("UR")
		r.Keywords = e.Fields["KW"]
		return r
	}
	if e.Format == "crossref" {
		r.Title = e.Field("title")
		r.Authors = e.Fields["author"]
		r.Publication = e.Field("journal")
		r.Year = e.Field("year")
		r.DOI = NormalizeDOI(e.Field("doi"))
		r.URL = e.Field("url")
		r.Abstract = e.Field("abstract")
		r.Keywords = e.Fields["keywords"]
		return r
	}
	r.Title = Plain(e.Field("title"))
	for _, a := range splitAuthors(e.Field("author")) {
		r.Authors = append(r.Authors, flipName(Plain(a)))
	}
	r.Abstract = Plain(e.Field("abstract"))
	r.DOI = NormalizeDOI(e.Field("doi"))
	r.Publication = Plain(firstNonEmpty(e.Field("journal"), e.Field("journaltitle"), e.Field("booktitle"), e.Field("publisher")))
	r.Year = yearOf(firstNonEmpty(e.Field("year"), e.Field("date")))
	r.URL = e.Field("url")
	for _, k := range strings.FieldsFunc(e.Field("keywords"), func(c rune) bool { return c == ',' || c == ';' }) {
		if k = strings.TrimSpace(Plain(k)); k != "" {
			r.Keywords = append(r.Keywords, k)
		}
	}
	return r
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func yearOf(s string) string {
	for i := 0; i+4 <= len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' && s[i+1] >= '0' && s[i+1] <= '9' && s[i+2] >= '0' && s[i+2] <= '9' && s[i+3] >= '0' && s[i+3] <= '9' {
			return s[i : i+4]
		}
	}
	return ""
}

// NormalizeDOI strips resolver prefixes and whitespace.
func NormalizeDOI(doi string) string {
	doi = strings.TrimSpace(doi)
	lower := strings.ToLower(doi)
	for _, p := range []string{"https://doi.org/", "http://doi.org/", "https://dx.doi.org/", "http://dx.doi.org/", "doi:"} {
		if strings.HasPrefix(lower, p) {
			return strings.TrimSpace(doi[len(p):])
		}
	}
	return doi
}

func splitAuthors(s string) []string {
	var out []string
	depth, last := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		}
		if depth == 0 && i+5 <= len(s) && strings.EqualFold(s[i:i+5], " and ") {
			out = append(out, strings.TrimSpace(s[last:i]))
			last = i + 5
			i += 4
		}
	}
	if tail := strings.TrimSpace(s[last:]); tail != "" {
		out = append(out, tail)
	}
	return out
}

// flipName turns "Last, First" into "First Last"; other forms are unchanged.
func flipName(n string) string {
	if last, first, ok := strings.Cut(n, ","); ok && !strings.Contains(first, ",") {
		return strings.TrimSpace(first) + " " + strings.TrimSpace(last)
	}
	return strings.TrimSpace(n)
}

func splitName(n string) (first, last string) {
	n = strings.TrimSpace(n)
	i := strings.LastIndexByte(n, ' ')
	if i < 0 {
		return "", n
	}
	return strings.TrimSpace(n[:i]), strings.TrimSpace(n[i+1:])
}

func initials(first string) string {
	var parts []string
	for _, f := range strings.Fields(first) {
		parts = append(parts, string([]rune(f)[0:1])+".")
	}
	return strings.Join(parts, " ")
}

// GenerateKey builds a citation key such as "smith2020" and makes it unique within used.
func GenerateKey(r Reference, used map[string]bool) string {
	base := "ref"
	if len(r.Authors) > 0 {
		_, last := splitName(r.Authors[0])
		var b strings.Builder
		for _, c := range strings.ToLower(last) {
			if unicode.IsLetter(c) && c < 128 {
				b.WriteRune(c)
			}
		}
		if b.Len() > 0 {
			base = b.String()
		}
	}
	base += r.Year
	key := base
	for n := 'a'; used[key]; n++ {
		key = base + string(n)
	}
	used[key] = true
	return key
}

func bibEscape(s string) string {
	if strings.Count(s, "{") != strings.Count(s, "}") {
		s = strings.NewReplacer("{", "", "}", "").Replace(s)
	}
	return s
}

// FormatBibTeX writes references as BibTeX. Unedited BibTeX imports are emitted
// verbatim; everything else is generated from the stored fields.
func FormatBibTeX(refs []Reference) string {
	used := map[string]bool{}
	for _, r := range refs {
		if r.Key != "" {
			used[r.Key] = true
		}
	}
	var b strings.Builder
	for _, r := range refs {
		if r.Format == "bibtex" && r.Raw != "" && !r.Edited {
			b.WriteString(r.Raw + "\n\n")
			continue
		}
		key := r.Key
		if key == "" {
			key = GenerateKey(r, used)
		}
		typ := r.EntryType
		if typ == "" || r.Format == "ris" {
			typ = "article"
		}
		fmt.Fprintf(&b, "@%s{%s,\n", typ, key)
		field := func(name, value string) {
			if strings.TrimSpace(value) != "" {
				fmt.Fprintf(&b, "  %s = {%s},\n", name, bibEscape(value))
			}
		}
		var authors []string
		for _, a := range r.Authors {
			f, l := splitName(a)
			if f == "" {
				authors = append(authors, l)
			} else {
				authors = append(authors, l+", "+f)
			}
		}
		field("title", r.Title)
		field("author", strings.Join(authors, " and "))
		field("journal", r.Publication)
		field("year", r.Year)
		field("doi", r.DOI)
		field("url", r.URL)
		field("keywords", strings.Join(r.Keywords, ", "))
		field("abstract", r.Abstract)
		b.WriteString("}\n\n")
	}
	return b.String()
}

// FormatRIS writes references as RIS, verbatim for unedited RIS imports.
func FormatRIS(refs []Reference) string {
	var b strings.Builder
	for _, r := range refs {
		if r.Format == "ris" && r.Raw != "" && !r.Edited {
			b.WriteString(r.Raw + "\n\n")
			continue
		}
		b.WriteString("TY  - JOUR\n")
		line := func(tag, v string) {
			if strings.TrimSpace(v) != "" {
				fmt.Fprintf(&b, "%s  - %s\n", tag, v)
			}
		}
		line("TI", r.Title)
		for _, a := range r.Authors {
			f, l := splitName(a)
			if f == "" {
				line("AU", l)
			} else {
				line("AU", l+", "+f)
			}
		}
		line("JO", r.Publication)
		line("PY", r.Year)
		line("DO", r.DOI)
		line("UR", r.URL)
		line("AB", r.Abstract)
		for _, k := range r.Keywords {
			line("KW", k)
		}
		b.WriteString("ER  - \n\n")
	}
	return b.String()
}

// FormatBibliography renders a sorted reference list. Supported styles: apa, ieee, plain.
func FormatBibliography(refs []Reference, style string) (string, error) {
	switch style {
	case "apa", "ieee", "plain":
	default:
		return "", fmt.Errorf("unsupported style %q", style)
	}
	items := append([]Reference(nil), refs...)
	if style != "ieee" {
		sort.SliceStable(items, func(i, j int) bool {
			return strings.ToLower(firstAuthorLast(items[i])+items[i].Year) < strings.ToLower(firstAuthorLast(items[j])+items[j].Year)
		})
	}
	var b strings.Builder
	for i, r := range items {
		switch style {
		case "apa":
			b.WriteString(fmt.Sprintf("%s (%s). %s.", apaAuthors(r.Authors), orND(r.Year), strings.TrimRight(r.Title, ".")))
			if r.Publication != "" {
				b.WriteString(" " + r.Publication + ".")
			}
		case "ieee":
			b.WriteString(fmt.Sprintf("[%d] %s, \"%s,\"", i+1, ieeeAuthors(r.Authors), strings.TrimRight(r.Title, ".")))
			if r.Publication != "" {
				b.WriteString(" " + r.Publication + ",")
			}
			b.WriteString(" " + orND(r.Year) + ".")
		default:
			b.WriteString(fmt.Sprintf("%s. %s. %s %s.", strings.Join(r.Authors, ", "), strings.TrimRight(r.Title, "."), r.Publication, r.Year))
		}
		if r.DOI != "" {
			b.WriteString(" https://doi.org/" + r.DOI)
		} else if r.URL != "" {
			b.WriteString(" " + r.URL)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

func orND(y string) string {
	if y == "" {
		return "n.d."
	}
	return y
}

func firstAuthorLast(r Reference) string {
	if len(r.Authors) == 0 {
		return r.Title
	}
	_, l := splitName(r.Authors[0])
	return l
}

func apaAuthors(authors []string) string {
	var out []string
	for _, a := range authors {
		f, l := splitName(a)
		if f == "" {
			out = append(out, l)
		} else {
			out = append(out, l+", "+initials(f))
		}
	}
	switch len(out) {
	case 0:
		return "Anonymous"
	case 1:
		return out[0]
	}
	return strings.Join(out[:len(out)-1], ", ") + ", & " + out[len(out)-1]
}

func ieeeAuthors(authors []string) string {
	var out []string
	for _, a := range authors {
		f, l := splitName(a)
		if f == "" {
			out = append(out, l)
		} else {
			out = append(out, initials(f)+" "+l)
		}
	}
	switch len(out) {
	case 0:
		return "Anonymous"
	case 1, 2:
		return strings.Join(out, " and ")
	}
	return out[0] + " et al."
}
