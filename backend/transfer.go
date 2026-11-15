package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"sefirot/internal/indexing"
	"sefirot/internal/vaultfs"
)

var (
	maxArchiveBytes   int64 = 2 << 30
	maxEntryBytes     int64 = 128 << 20
	maxExpandedBytes  int64 = 2 << 30
	maxArchiveEntries       = 100000
)

func (a *App) handleExport(w http.ResponseWriter, r *http.Request) {
	tmp, err := os.CreateTemp("", "sefirot-export-*.zip")
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := a.writeArchive(zip.NewWriter(tmp), tmp); err != nil {
		log.Printf("vault export: %v", err)
		jsonError(w, 500, fmt.Errorf("export failed: %w", err))
		return
	}
	info, err := tmp.Stat()
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="sefirot-archive.zip"`)
	http.ServeContent(w, r, "sefirot-archive.zip", info.ModTime(), tmp)
}

func (a *App) writeArchive(zw *zip.Writer, out io.Writer) error {
	var walk func(string) error
	walk = func(dir string) error {
		entries, err := indexing.ReadDir(a.root, dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			rel := entry.Name()
			if dir != "." {
				rel = path.Join(dir, entry.Name())
			}
			if indexing.IsReserved(rel) || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			if entry.IsDir() {
				if err := walk(rel); err != nil {
					return err
				}
				continue
			}
			if err := a.addToArchive(zw, rel); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk("."); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

func (a *App) addToArchive(zw *zip.Writer, rel string) error {
	f, err := a.root.Open(rel)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if info.Size() > maxEntryBytes {
		return fmt.Errorf("%s exceeds the %d MiB per-file export limit", rel, maxEntryBytes>>20)
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = rel
	header.Method = zip.Deflate
	dst, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, f)
	return err
}

type importReport struct {
	Imported    []string `json:"imported"`
	Skipped     []string `json:"skipped"`
	IndexErrors []string `json:"indexErrors"`
	Count       int      `json:"count"`
}

type limitedCounter struct {
	r     io.Reader
	n     int64
	limit int64
}

func (l *limitedCounter) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	l.n += int64(n)
	if l.n > l.limit {
		return n, errors.New("archive entry exceeds size limit")
	}
	return n, err
}

func (a *App) handleImport(w http.ResponseWriter, r *http.Request) {
	tmp, err := os.CreateTemp("", "sefirot-import-*.zip")
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	n, err := io.Copy(tmp, io.LimitReader(r.Body, maxArchiveBytes+1))
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	if n > maxArchiveBytes {
		jsonError(w, 413, fmt.Errorf("archive exceeds %d GiB limit", maxArchiveBytes>>30))
		return
	}
	reader, err := zip.NewReader(tmp, n)
	if err != nil {
		jsonError(w, 400, fmt.Errorf("invalid ZIP archive: %w", err))
		return
	}
	if len(reader.File) > maxArchiveEntries {
		jsonError(w, 413, fmt.Errorf("archive has more than %d entries", maxArchiveEntries))
		return
	}
	// Validate every entry before writing anything so an unsafe archive changes nothing.
	type planned struct {
		file *zip.File
		rel  string
	}
	var plan []planned
	report := importReport{Imported: []string{}, Skipped: []string{}, IndexErrors: []string{}}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			jsonError(w, 400, errors.New("symbolic links are not allowed in vault imports"))
			return
		}
		rel, perr := indexing.SafeRelativePath(file.Name)
		if perr != nil || strings.Contains(file.Name, `\`) || rel != file.Name {
			jsonError(w, 400, fmt.Errorf("unsafe archive path: %q", file.Name))
			return
		}
		lower := strings.ToLower(rel)
		if strings.EqualFold(path.Base(rel), "index.db") || strings.HasSuffix(lower, ".db-wal") || strings.HasSuffix(lower, ".db-shm") {
			report.Skipped = append(report.Skipped, rel)
			continue
		}
		if file.UncompressedSize64 > uint64(maxEntryBytes) {
			jsonError(w, 413, fmt.Errorf("archive entry %s exceeds %d MiB limit", rel, maxEntryBytes>>20))
			return
		}
		plan = append(plan, planned{file, rel})
	}
	var created []string
	rollback := func() {
		for _, rel := range created {
			_ = a.root.Remove(rel)
		}
	}
	var expanded int64
	for _, item := range plan {
		if _, openErr := a.root.Lstat(item.rel); openErr == nil {
			report.Skipped = append(report.Skipped, item.rel)
			continue
		} else if !errors.Is(openErr, os.ErrNotExist) {
			rollback()
			jsonError(w, 400, openErr)
			return
		}
		src, err := item.file.Open()
		if err != nil {
			rollback()
			jsonError(w, 400, err)
			return
		}
		data, readErr := io.ReadAll(&limitedCounter{r: src, limit: maxEntryBytes})
		_ = src.Close()
		if readErr != nil {
			rollback()
			jsonError(w, 413, fmt.Errorf("%s: %w", item.rel, readErr))
			return
		}
		expanded += int64(len(data))
		if expanded > maxExpandedBytes {
			rollback()
			jsonError(w, 413, fmt.Errorf("expanded archive exceeds %d GiB limit", maxExpandedBytes>>30))
			return
		}
		ok, err := vaultfs.WriteExclusive(a.root, item.rel, data)
		if err != nil {
			rollback()
			jsonError(w, 500, err)
			return
		}
		if !ok {
			report.Skipped = append(report.Skipped, item.rel)
			continue
		}
		created = append(created, item.rel)
		report.Imported = append(report.Imported, item.rel)
	}
	for _, rel := range created {
		if strings.EqualFold(path.Ext(rel), ".html") {
			if err := indexing.IndexFile(a.root, a.db, rel); err != nil {
				report.IndexErrors = append(report.IndexErrors, rel+": "+err.Error())
			}
		}
	}
	if _, err := a.rebuildResearch(r.Context()); err != nil {
		report.IndexErrors = append(report.IndexErrors, "research: "+err.Error())
	}
	report.Count = len(report.Imported)
	writeJSON(w, report)
}
