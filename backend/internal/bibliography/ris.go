package bibliography

import (
	"strings"
)

// ParseRIS reads RIS records (TY ... ER). Each record keeps its verbatim text.
func ParseRIS(text string) ([]Entry, []error) {
	var entries []Entry
	var errs []error
	var cur *Entry
	var raw strings.Builder
	lastTag := ""
	flush := func() {
		if cur != nil {
			cur.Raw = strings.TrimSpace(raw.String())
			entries = append(entries, *cur)
		}
		cur = nil
		raw.Reset()
		lastTag = ""
	}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		tag, value, ok := splitRIS(line)
		if !ok {
			if cur != nil && lastTag != "" {
				vals := cur.Fields[lastTag]
				if len(vals) > 0 {
					vals[len(vals)-1] += " " + strings.TrimSpace(line)
				}
				raw.WriteString(line + "\n")
			}
			continue
		}
		switch tag {
		case "TY":
			flush()
			cur = &Entry{Format: "ris", Type: strings.ToLower(value), Fields: map[string][]string{}}
		case "ER":
			if cur != nil {
				raw.WriteString(line + "\n")
			}
			flush()
			continue
		default:
			if cur == nil {
				continue
			}
			cur.Fields[tag] = append(cur.Fields[tag], value)
			lastTag = tag
		}
		raw.WriteString(line + "\n")
	}
	if cur != nil {
		errs = append(errs, errUnterminated)
		flush()
	}
	for i := range entries {
		entries[i].Key = entries[i].Field("ID")
	}
	return entries, errs
}

type risError string

func (e risError) Error() string { return string(e) }

const errUnterminated = risError("RIS record without ER terminator was imported as is")

func splitRIS(line string) (string, string, bool) {
	if len(line) >= 5 && line[2] == ' ' && line[3] == ' ' && line[4] == '-' {
		tag := line[:2]
		if tag[0] >= 'A' && tag[0] <= 'Z' && (tag[1] >= 'A' && tag[1] <= 'Z' || tag[1] >= '0' && tag[1] <= '9') {
			return tag, strings.TrimSpace(strings.TrimPrefix(line[5:], " ")), true
		}
	}
	return "", "", false
}
