package indexing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"sefirot/internal/database"
	"sort"
	"strings"
	"sync"
	"time"
)

func ReadRootFile(root *os.Root, name string) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("vault note is not a regular file")
	}
	if info.Size() > 32<<20 {
		return nil, fmt.Errorf("HTML note exceeds 32 MiB indexing limit")
	}
	return io.ReadAll(io.LimitReader(f, (32<<20)+1))
}

func IndexFile(root *os.Root, db *database.Database, rel string) error {
	return indexFile(root, db, rel, false)
}

func indexFile(root *os.Root, db *database.Database, rel string, force bool) error {
	if !strings.EqualFold(path.Ext(rel), ".html") {
		return nil
	}
	raw, err := ReadRootFile(root, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return db.Delete(context.Background(), rel)
		}
		return err
	}
	hash := sha256.Sum256(raw)
	contentHash := hex.EncodeToString(hash[:])
	if !force {
		if same, hashErr := db.HasHash(context.Background(), rel, contentHash); hashErr != nil {
			return hashErr
		} else if same {
			return nil
		}
	}
	updatedAt := ""
	if info, statErr := root.Stat(rel); statErr == nil {
		updatedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
	}
	clean := StripSourceComment(raw)
	title, plain, links, relationships, metadata, err := ExtractHTMLDetails(clean)
	if err == nil && (title == "" || title == "Untitled") {
		title = strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	}
	if err != nil {
		return err
	}
	if source, isMarkdown := ExtractMarkdownSource(raw); isMarkdown {
		front, _ := ParseFrontMatter(source)
		metadata.Aliases = append(metadata.Aliases, front.list("aliases")...)
		metadata.Tags = append(metadata.Tags, front.list("tags")...)
		if metadata.DocType == "" {
			metadata.DocType = front.first("type")
		}
		if metadata.Description == "" {
			metadata.Description = front.first("description")
		}
		if t := front.first("title"); t != "" {
			title = t
		}
	}
	summary := metadata.Summary
	if summary == "" {
		summary = database.SummarizeDocument(plain)
	}
	shortSummary := database.SummarizeWords(summary, 18)
	detailedSummary := database.SummarizeWords(plain, 120)
	return db.Upsert(context.Background(), database.IndexedNote{Path: rel, Title: title, PlainText: plain, Summary: summary, SummaryShort: shortSummary, SummaryDetail: detailedSummary, Headings: metadata.Headings, Description: metadata.Description, DocType: metadata.DocType, Tags: metadata.Tags, Aliases: metadata.Aliases, ContentHash: contentHash, ContentHTML: string(raw), Links: links, Relationships: relationships, UpdatedAt: updatedAt})
}

func RebuildIndex(root *os.Root, db *database.Database) error {
	force := db.ReindexRequired(context.Background())
	failures := 0
	existing := map[string]struct{}{}
	var walk func(string) error
	walk = func(dir string) error {
		entries, err := ReadDir(root, dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			rel := entry.Name()
			if dir != "." && dir != "" {
				rel = path.Join(dir, entry.Name())
			}
			if IsReserved(rel) {
				continue
			}
			if entry.IsDir() {
				if err := walk(rel); err != nil {
					return err
				}
				continue
			}
			if !strings.EqualFold(path.Ext(entry.Name()), ".html") {
				continue
			}
			existing[rel] = struct{}{}
			if err := indexFile(root, db, rel, force); err != nil && !os.IsNotExist(err) {
				failures++
				log.Printf("index %s: %v", rel, err)
			}
		}
		return nil
	}
	if err := walk("."); err != nil {
		return err
	}
	if err := db.PruneMissing(context.Background(), existing); err != nil {
		return err
	}
	if force && failures == 0 {
		return db.ClearReindexRequired(context.Background())
	}
	return nil
}

type VaultWatcher struct {
	watcher *fsnotify.Watcher
	done    chan struct{}
	once    sync.Once
	wg      sync.WaitGroup
	timerWG sync.WaitGroup
}

func (v *VaultWatcher) Close() error {
	var err error
	v.once.Do(func() { close(v.done); err = v.watcher.Close(); v.wg.Wait(); v.timerWG.Wait() })
	return err
}

func StartWatcher(root *os.Root, db *database.Database, vaultDir string) (*VaultWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	v := &VaultWatcher{watcher: w, done: make(chan struct{})}
	vaultPath, err := filepath.Abs(vaultDir)
	if err != nil {
		_ = w.Close()
		return nil, err
	}
	addTree := func(start string) {
		_ = filepath.WalkDir(start, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return nil
			}
			if d.IsDir() {
				if relPath, relErr := filepath.Rel(vaultPath, p); relErr == nil && IsReserved(filepath.ToSlash(relPath)) {
					return filepath.SkipDir
				}
				if err := w.Add(p); err != nil {
					fmt.Printf("watch %s: %v\n", p, err)
				}
			}
			return nil
		})
	}
	addTree(vaultPath)
	var mu sync.Mutex
	timers := map[string]*time.Timer{}
	v.wg.Add(2)
	go func() {
		defer v.wg.Done()
		for {
			select {
			case <-v.done:
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Op&fsnotify.Create != 0 {
					rel, relErr := filepath.Rel(vaultPath, ev.Name)
					if relErr == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
						if f, openErr := root.Open(filepath.ToSlash(rel)); openErr == nil {
							info, statErr := f.Stat()
							_ = f.Close()
							if statErr == nil && info.IsDir() {
								addTree(ev.Name)
							}
						}
					}
				}
				if relEvent, relErr := filepath.Rel(vaultPath, ev.Name); relErr == nil && IsReserved(filepath.ToSlash(relEvent)) {
					continue
				}
				if !strings.EqualFold(filepath.Ext(ev.Name), ".html") || ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
					continue
				}
				name := ev.Name
				mu.Lock()
				if old := timers[name]; old != nil {
					if old.Stop() {
						v.timerWG.Done()
					}
				}
				v.timerWG.Add(1)
				var timer *time.Timer
				timer = time.AfterFunc(150*time.Millisecond, func() {
					defer v.timerWG.Done()
					rel, e := filepath.Rel(vaultPath, name)
					if e == nil {
						if e = IndexFile(root, db, filepath.ToSlash(rel)); e != nil && !os.IsNotExist(e) {
							fmt.Printf("index watcher: %v\n", e)
						}
					}
					mu.Lock()
					if timers[name] == timer {
						delete(timers, name)
					}
					mu.Unlock()
				})
				timers[name] = timer
				mu.Unlock()
			case e, ok := <-w.Errors:
				if !ok {
					return
				}
				fmt.Printf("watcher: %v\n", e)
				if errors.Is(e, fsnotify.ErrEventOverflow) {
					if rebuildErr := RebuildIndex(root, db); rebuildErr != nil {
						fmt.Printf("index overflow recovery: %v\n", rebuildErr)
					}
				}
			}
		}
	}()
	go func() {
		defer v.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-v.done:
				mu.Lock()
				for _, t := range timers {
					if t.Stop() {
						v.timerWG.Done()
					}
				}
				timers = map[string]*time.Timer{}
				mu.Unlock()
				return
			case <-ticker.C:
				if err := RebuildIndex(root, db); err != nil {
					fmt.Printf("index reconciliation: %v\n", err)
				}
			}
		}
	}()
	return v, nil
}

func ReadDir(root *os.Root, name string) ([]os.DirEntry, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name()) })
	return entries, nil
}
