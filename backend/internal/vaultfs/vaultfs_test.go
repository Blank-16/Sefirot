package vaultfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newRoot(t *testing.T) (*os.Root, string) {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return root, dir
}

func TestWriteAtomicReplacesWithoutResidue(t *testing.T) {
	root, dir := newRoot(t)
	if err := WriteAtomic(root, "a/b.html", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(root, "a/b.html", []byte("two")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a", "b.html"))
	if string(got) != "two" {
		t.Fatalf("content = %q", got)
	}
	if left, _ := os.ReadDir(filepath.Join(dir, ".sefirot", "tmp")); len(left) != 0 {
		t.Fatalf("staging residue: %v", left)
	}
}

func TestMoveRefusesToOverwrite(t *testing.T) {
	root, dir := newRoot(t)
	os.WriteFile(filepath.Join(dir, "x.html"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "y.html"), []byte("y"), 0o644)
	if err := Move(root, "x.html", "y.html"); !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "y.html")); string(got) != "y" {
		t.Fatal("destination was modified")
	}
}

func TestTrashRestorePurgeAndExpiry(t *testing.T) {
	root, dir := newRoot(t)
	os.WriteFile(filepath.Join(dir, "n.html"), []byte("note"), 0o644)
	entry, err := Trash(root, "n.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "n.html")); !os.IsNotExist(err) {
		t.Fatal("note still in vault")
	}
	if _, err := RestoreTrash(root, "../../etc", ""); err == nil {
		t.Fatal("invalid id must be rejected")
	}
	if _, err := RestoreTrash(root, entry.ID, "../escape.html"); err == nil {
		t.Fatal("restore target must stay inside the vault")
	}
	if _, err := RestoreTrash(root, entry.ID, ".sefirot/inside.html"); err == nil {
		t.Fatal("restore into reserved directory must be rejected")
	}
	if n, _ := PurgeExpiredTrash(root, time.Now()); n != 0 {
		t.Fatal("fresh entries must not expire")
	}
	if n, _ := PurgeExpiredTrash(root, time.Now().Add(TrashRetention+time.Hour)); n != 1 {
		t.Fatalf("expired purge = %d", n)
	}
	if items, _ := ListTrash(root); len(items) != 0 {
		t.Fatalf("trash not empty: %v", items)
	}
}
