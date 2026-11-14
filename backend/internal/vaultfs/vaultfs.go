package vaultfs

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"time"

	"sefirot/internal/indexing"
)

const (
	tmpDir  = indexing.ReservedDir + "/tmp"
	perm    = 0o644
	dirPerm = 0o755
)

var ErrExists = errors.New("destination already exists")

func randomName() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func ensureTmp(root *os.Root) error {
	return indexing.EnsureRootDirs(root, tmpDir+"/x")
}

func syncParent(root *os.Root, rel string) {
	dir := path.Dir(rel)
	f, err := root.Open(dir)
	if err != nil {
		return
	}
	_ = f.Sync()
	_ = f.Close()
}

func renameWithRetry(root *os.Root, from, to string) error {
	attempts := 1
	if runtime.GOOS == "windows" {
		attempts = 6
	}
	var err error
	for i := 0; i < attempts; i++ {
		if err = root.Rename(from, to); err == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 15 * time.Millisecond)
	}
	return err
}

func stage(root *os.Root, data []byte) (string, error) {
	if err := ensureTmp(root); err != nil {
		return "", err
	}
	name, err := randomName()
	if err != nil {
		return "", err
	}
	tmp := tmpDir + "/" + name + ".tmp"
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return "", err
	}
	n, werr := f.Write(data)
	if werr == nil && n != len(data) {
		werr = io.ErrShortWrite
	}
	if werr == nil {
		werr = f.Sync()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = root.Remove(tmp)
		return "", werr
	}
	return tmp, nil
}

// WriteAtomic replaces rel with data so that readers and crashes observe either
// the complete old file or the complete new file, never a truncated one.
func WriteAtomic(root *os.Root, rel string, data []byte) error {
	if err := indexing.EnsureRootDirs(root, rel); err != nil {
		return err
	}
	tmp, err := stage(root, data)
	if err != nil {
		return err
	}
	if err := renameWithRetry(root, tmp, rel); err != nil {
		_ = root.Remove(tmp)
		return err
	}
	syncParent(root, rel)
	return nil
}

// WriteExclusive creates rel only if it does not exist. The bool reports whether it was created.
func WriteExclusive(root *os.Root, rel string, data []byte) (bool, error) {
	if err := indexing.EnsureRootDirs(root, rel); err != nil {
		return false, err
	}
	f, err := root.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	n, werr := f.Write(data)
	if werr == nil && n != len(data) {
		werr = io.ErrShortWrite
	}
	if werr == nil {
		werr = f.Sync()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = root.Remove(rel)
		return false, werr
	}
	syncParent(root, rel)
	return true, nil
}

// Move renames from to to without overwriting an existing destination.
func Move(root *os.Root, from, to string) error {
	if from == to {
		return nil
	}
	if _, err := root.Lstat(to); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, to)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := indexing.EnsureRootDirs(root, to); err != nil {
		return err
	}
	if err := renameWithRetry(root, from, to); err != nil {
		return err
	}
	syncParent(root, to)
	return nil
}

// CleanTmp removes staging files left by an interrupted write.
func CleanTmp(root *os.Root) {
	entries, err := indexing.ReadDir(root, tmpDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		_ = root.Remove(tmpDir + "/" + e.Name())
	}
}
