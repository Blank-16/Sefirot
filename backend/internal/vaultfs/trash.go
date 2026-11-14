package vaultfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"sefirot/internal/indexing"
)

const (
	trashDir       = indexing.ReservedDir + "/trash"
	TrashRetention = 30 * 24 * time.Hour
)

type TrashEntry struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	DeletedAt string `json:"deletedAt"`
	Size      int64  `json:"size"`
}

func trashPayload(id string) string { return trashDir + "/" + id + "/content" }
func trashMeta(id string) string    { return trashDir + "/" + id + "/entry.json" }

func validID(id string) bool {
	if len(id) < 8 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Trash moves rel into the vault trash so a deletion can be undone.
func Trash(root *os.Root, rel string) (TrashEntry, error) {
	info, err := root.Lstat(rel)
	if err != nil {
		return TrashEntry{}, err
	}
	if !info.Mode().IsRegular() {
		return TrashEntry{}, fmt.Errorf("only regular files can be trashed")
	}
	name, err := randomName()
	if err != nil {
		return TrashEntry{}, err
	}
	entry := TrashEntry{ID: name, Path: rel, DeletedAt: time.Now().UTC().Format(time.RFC3339), Size: info.Size()}
	meta, err := json.Marshal(entry)
	if err != nil {
		return TrashEntry{}, err
	}
	if err := WriteAtomic(root, trashMeta(entry.ID), meta); err != nil {
		return TrashEntry{}, err
	}
	if err := renameWithRetry(root, rel, trashPayload(entry.ID)); err != nil {
		_ = root.Remove(trashMeta(entry.ID))
		_ = root.Remove(trashDir + "/" + entry.ID)
		return TrashEntry{}, err
	}
	return entry, nil
}

func readEntry(root *os.Root, id string) (TrashEntry, error) {
	var entry TrashEntry
	raw, err := indexing.ReadRootFile(root, trashMeta(id))
	if err != nil {
		return entry, err
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return entry, err
	}
	if entry.ID != id {
		return entry, errors.New("corrupt trash entry")
	}
	if _, err := indexing.SafeRelativePath(entry.Path); err != nil {
		return entry, err
	}
	return entry, nil
}

func ListTrash(root *os.Root) ([]TrashEntry, error) {
	dirs, err := indexing.ReadDir(root, trashDir)
	if errors.Is(err, os.ErrNotExist) {
		return []TrashEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []TrashEntry{}
	for _, d := range dirs {
		if !d.IsDir() || !validID(d.Name()) {
			continue
		}
		if e, err := readEntry(root, d.Name()); err == nil {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DeletedAt > out[j].DeletedAt })
	return out, nil
}

// RestoreTrash returns an entry to its original path, or to target when given.
// It never overwrites an existing file.
func RestoreTrash(root *os.Root, id, target string) (string, error) {
	if !validID(id) {
		return "", errors.New("invalid trash id")
	}
	entry, err := readEntry(root, id)
	if err != nil {
		return "", err
	}
	dest := entry.Path
	if target != "" {
		if dest, err = indexing.SafeRelativePath(target); err != nil {
			return "", err
		}
	}
	if err := Move(root, trashPayload(id), dest); err != nil {
		return "", err
	}
	_ = root.Remove(trashMeta(id))
	_ = root.Remove(trashDir + "/" + id)
	return dest, nil
}

func PurgeTrash(root *os.Root, id string) error {
	if !validID(id) {
		return errors.New("invalid trash id")
	}
	_ = root.Remove(trashPayload(id))
	_ = root.Remove(trashMeta(id))
	return root.Remove(trashDir + "/" + id)
}

// PurgeExpiredTrash permanently removes entries older than TrashRetention.
func PurgeExpiredTrash(root *os.Root, now time.Time) (int, error) {
	entries, err := ListTrash(root)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		at, perr := time.Parse(time.RFC3339, e.DeletedAt)
		if perr != nil || now.Sub(at) < TrashRetention {
			continue
		}
		if PurgeTrash(root, e.ID) == nil {
			n++
		}
	}
	return n, nil
}
