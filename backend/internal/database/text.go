package database

import (
	"strings"
)

func SummarizeDocument(plain string) string { return SummarizeWords(plain, 42) }

func SummarizeWords(plain string, limit int) string {
	fields := strings.Fields(strings.TrimSpace(plain))
	if len(fields) == 0 {
		return "No summary available yet."
	}
	if limit > 0 && len(fields) > limit {
		fields = fields[:limit]
		return strings.TrimRight(strings.Join(fields, " "), ".,;:") + "…"
	}
	return strings.Join(fields, " ")
}
