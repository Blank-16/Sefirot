package main

import (
	"context"
	"crypto/rand"

	"embed"
	"encoding/hex"

	"errors"
	"flag"
	"fmt"

	"log"

	"net"
	"net/http"

	"os"
	"os/signal"

	"path/filepath"

	"strconv"

	"sync"
	"time"

	"sefirot/internal/bibliography"
	"sefirot/internal/database"
	"sefirot/internal/indexing"
	"sefirot/internal/instance"
	"sefirot/internal/semantic"
	"sefirot/internal/vaultfs"
)

//go:embed web
var webFiles embed.FS

type App struct {
	root          *os.Root
	vaultDir      string
	port          int
	baseURL       string
	db            *database.Database
	embeddings    *semantic.EmbeddingService
	terminalToken string
	terminalMu    sync.Mutex
	terminals     map[string]*terminalSession
	shutdown      chan struct{}
	shutdownOnce  sync.Once
	settings      *settingsStore
	crossref      bibliography.Crossref
}

type noteRequest struct {
	Path         string `json:"path"`
	Content      string `json:"content"`
	IsMarkdown   bool   `json:"isMarkdown"`
	CreateOnly   bool   `json:"createOnly,omitempty"`
	ExpectedHash string `json:"expectedHash,omitempty"`
	Force        bool   `json:"force,omitempty"`
}
type renameRequest struct {
	OldPath string `json:"oldPath"`
	NewPath string `json:"newPath"`
}
type NoteResponse struct {
	Path          string                     `json:"path"`
	Title         string                     `json:"title"`
	Content       string                     `json:"content"`
	RenderedHTML  string                     `json:"renderedHtml"`
	IsMarkdown    bool                       `json:"isMarkdown"`
	Backlinks     []database.Backlink        `json:"backlinks"`
	Summary       string                     `json:"summary"`
	SummaryShort  string                     `json:"summaryShort"`
	SummaryDetail string                     `json:"summaryDetail"`
	Headings      []database.DocumentHeading `json:"headings"`
	Description   string                     `json:"description"`
	DocType       string                     `json:"type"`
	Tags          []string                   `json:"tags"`
	ContentHash   string                     `json:"contentHash"`
	UpdatedAt     string                     `json:"updatedAt"`
}
type TreeNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Type     string     `json:"type"`
	Title    string     `json:"title,omitempty"`
	Children []TreeNode `json:"children,omitempty"`
}

func main() {
	dataDirFlag := flag.String("data-dir", "", "SEFIROT data directory (contains vault/ and index.db)")
	vaultDirFlag := flag.String("vault-dir", "", "override the vault directory")
	dbPathFlag := flag.String("db-path", "", "override the SQLite database path")
	portFlag := flag.Int("port", 8080, "loopback HTTP port; 0 selects an available port")
	readyFile := flag.String("ready-file", "", "write the loopback URL and shutdown token to this file (desktop wrapper)")
	flag.Parse()

	paths, err := resolveAppPaths(*dataDirFlag, *vaultDirFlag, *dbPathFlag)
	if err != nil {
		log.Fatal(err)
	}
	if warning := instance.PathWarning(paths.dataDir); warning != "" {
		log.Printf("WARNING: %s", warning)
	}
	if err := os.MkdirAll(paths.dataDir, 0755); err != nil {
		log.Fatal(err)
	}
	if info, err := os.Lstat(paths.vaultDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			log.Fatal("vault directory must be a real directory, not a symlink or file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Fatal(err)
	} else if err := os.MkdirAll(paths.vaultDir, 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.dbPath), 0755); err != nil {
		log.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Dir(paths.dbPath))
	if err != nil {
		log.Fatal(err)
	}
	defer lock.Release()
	go func() {
		<-lock.Lost()
		log.Fatal("another SEFIROT process took over this data directory; exiting to protect the database")
	}()
	root, err := os.OpenRoot(paths.vaultDir)
	if err != nil {
		log.Fatal(err)
	}
	db, err := database.OpenDatabase(paths.dbPath)
	if err != nil {
		_ = root.Close()
		log.Fatal(err)
	}
	defer db.Close()
	defer root.Close()

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(*portFlag)))
	if err != nil && *portFlag == 8080 {
		log.Printf("port 8080 is unavailable (%v); selecting an ephemeral loopback port", err)
		listener, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		log.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Fatal(err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	app := &App{root: root, vaultDir: paths.vaultDir, port: port, baseURL: baseURL, db: db, settings: &settingsStore{path: filepath.Join(filepath.Dir(paths.dbPath), "settings.json")}, terminalToken: hex.EncodeToString(tokenBytes), terminals: make(map[string]*terminalSession), shutdown: make(chan struct{})}
	app.embeddings = semantic.NewEmbeddingService(db)
	ctx, stop := signal.NotifyContext(context.Background(), terminationSignals()...)
	defer stop()
	defer app.embeddings.Close()
	if err := app.seed(); err != nil {
		log.Fatal(err)
	}
	if err := app.seedAgentGuides(); err != nil {
		log.Fatal(err)
	}
	vaultfs.CleanTmp(root)
	if _, err := vaultfs.PurgeExpiredTrash(root, time.Now()); err != nil {
		log.Printf("trash retention: %v", err)
	}
	if _, err := db.PruneOrphanRevisions(context.Background(), time.Now().Add(-vaultfs.TrashRetention)); err != nil {
		log.Printf("revision retention: %v", err)
	}
	if err := indexing.RebuildIndex(root, db); err != nil {
		log.Fatal(err)
	}
	if n, err := app.rebuildResearch(context.Background()); err != nil {
		log.Printf("research rebuild: %v", err)
	} else if n > 0 {
		log.Printf("restored %d research records from the vault", n)
	}
	watcher, err := indexing.StartWatcher(root, db, paths.vaultDir)
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()
	defer app.stopTerminals()
	app.embeddings.Start(ctx)

	server := &http.Server{Handler: app.routes(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("SEFIROT running at %s", baseURL)
	log.Printf("Vault: %s", paths.vaultDir)
	log.Printf("Database: %s", paths.dbPath)
	if *readyFile != "" {
		if err := os.MkdirAll(filepath.Dir(*readyFile), 0755); err != nil {
			log.Fatal(err)
		}
		readyContents := baseURL + "\n" + app.terminalToken
		if err := os.WriteFile(*readyFile, []byte(readyContents), 0600); err != nil {
			log.Fatal(err)
		}
	}
	go func() {
		select {
		case <-ctx.Done():
		case <-app.shutdown:
			stop()
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("HTTP server: %v", err)
	}
}

// Go 1.24's os.Root intentionally has no Rename method (it was added in Go 1.25).
// Copy-create-remove keeps every filesystem operation scoped to the open root.

func extractVisibleText(raw []byte) string {
	_, text, _, err := indexing.ExtractHTML(raw)
	if err != nil {
		return ""
	}
	return text
}

type restoreRequest struct {
	Path       string `json:"path"`
	RevisionID int64  `json:"revisionId"`
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", a.handleIndex)
	mux.HandleFunc("GET /app.js", a.handleJS)
	mux.HandleFunc("GET /js/{path...}", a.handleJSModule)
	mux.HandleFunc("GET /brand.svg", a.handleBrand)
	mux.HandleFunc("GET /visual-system.css", a.handleStyles)
	mux.HandleFunc("GET /tailwind.css", a.handleTailwindCSS)
	mux.HandleFunc("GET /vendor/{path...}", a.handleVendor)
	mux.HandleFunc("GET /vault-assets/{path...}", a.handleAsset)
	mux.HandleFunc("GET /api/tree", a.handleTree)
	mux.HandleFunc("GET /api/research", a.handleResearchList)
	mux.HandleFunc("POST /api/research", a.handleResearchCreate)
	mux.HandleFunc("GET /api/research/{id}", a.handleResearchGet)
	mux.HandleFunc("PUT /api/research/{id}", a.handleResearchUpdate)
	mux.HandleFunc("DELETE /api/research/{id}", a.handleResearchDelete)
	mux.HandleFunc("GET /api/research/{id}/citations", a.handleResearchCitations)
	mux.HandleFunc("POST /api/research/{id}/citations", a.handleResearchAddCitation)
	mux.HandleFunc("GET /api/research/{id}/annotations", a.handleResearchAnnotations)
	mux.HandleFunc("POST /api/research/{id}/annotations", a.handleResearchAddAnnotation)
	mux.HandleFunc("POST /api/research/{id}/pdf", a.handleResearchUploadPDF)
	mux.HandleFunc("DELETE /api/research/{id}/citations/{cid}", a.handleResearchDeleteCitation)
	mux.HandleFunc("DELETE /api/research/{id}/annotations/{aid}", a.handleResearchDeleteAnnotation)
	mux.HandleFunc("GET /api/notes", a.handleGetNote)
	mux.HandleFunc("POST /api/notes", a.handleSaveNote)
	mux.HandleFunc("POST /api/notes/rename", a.handleRename)
	mux.HandleFunc("POST /api/folders", a.handleCreateFolder)
	mux.HandleFunc("DELETE /api/notes", a.handleDeleteNote)
	mux.HandleFunc("GET /api/search", a.handleSearch)
	mux.HandleFunc("GET /api/graph", a.handleGraph)
	mux.HandleFunc("GET /api/ai/context", a.handleAIContext)
	mux.HandleFunc("GET /api/ai/search", a.handleAISearch)
	mux.HandleFunc("GET /api/tags", a.handleTags)
	mux.HandleFunc("GET /api/recent", a.handleRecent)
	mux.HandleFunc("GET /api/agents/runs", a.handleAgentRuns)
	mux.HandleFunc("GET /api/notes/revisions", a.handleRevisions)
	mux.HandleFunc("POST /api/notes/restore", a.handleRestoreRevision)
	mux.HandleFunc("GET /api/health", a.handleHealth)
	mux.HandleFunc("POST /api/shutdown", a.handleShutdown)
	mux.HandleFunc("GET /api/export", a.handleExport)
	mux.HandleFunc("POST /api/import", a.handleImport)
	mux.HandleFunc("GET /api/semantic/status", a.handleSemanticStatus)
	mux.HandleFunc("GET /api/semantic/search", a.handleSemanticSearch)
	mux.HandleFunc("GET /api/hybrid/search", a.handleHybridSearch)
	mux.HandleFunc("GET /api/terminal/config", a.handleTerminalConfig)
	mux.HandleFunc("POST /api/terminal/start", a.handleTerminalStart)
	mux.HandleFunc("GET /api/terminal/ws", a.handleTerminalWS)
	mux.HandleFunc("POST /api/terminal/stop", a.handleTerminalStop)
	mux.HandleFunc("POST /api/research/import", a.handleResearchImport)
	mux.HandleFunc("GET /api/research/export", a.handleResearchExport)
	mux.HandleFunc("GET /api/research/bibliography", a.handleResearchBibliography)
	mux.HandleFunc("POST /api/research/lookup-doi", a.handleResearchLookupDOI)
	mux.HandleFunc("GET /api/settings", a.handleSettingsGet)
	mux.HandleFunc("PUT /api/settings", a.handleSettingsPut)
	mux.HandleFunc("GET /api/integrity", a.handleIntegrity)
	mux.HandleFunc("GET /api/resolve", a.handleResolve)
	mux.HandleFunc("GET /api/trash", a.handleTrashList)
	mux.HandleFunc("POST /api/trash/restore", a.handleTrashRestore)
	mux.HandleFunc("DELETE /api/trash", a.handleTrashPurge)
	return securityHeaders(localOnly(mux, a.port))
}
