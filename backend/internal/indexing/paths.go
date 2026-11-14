package indexing

import (
	"fmt"
	"os"
	"path"
	"strings"
)

func SafeRelativePath(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\`, "/"))
	if raw == "" || strings.HasPrefix(raw, "/") || strings.Contains(raw, ":") || strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("invalid vault path")
	}
	clean := path.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path escapes vault")
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".." || part == "" {
			return "", fmt.Errorf("invalid path segment")
		}
	}
	if IsReserved(clean) {
		return "", fmt.Errorf("path is reserved for application data")
	}
	return clean, nil
}

func EnsureRootDirs(root *os.Root, rel string) error {
	dir := path.Dir(rel)
	if dir == "." {
		return nil
	}
	current := ""
	for _, part := range strings.Split(dir, "/") {
		current = path.Join(current, part)
		if err := root.Mkdir(current, 0755); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}

// ReservedDir holds application state (staging files, trash) inside the vault.
// It is never indexed, listed, exported, or served.
const ReservedDir = ".sefirot"

func IsReserved(rel string) bool {
	first, _, _ := strings.Cut(rel, "/")
	return strings.EqualFold(first, ReservedDir)
}
