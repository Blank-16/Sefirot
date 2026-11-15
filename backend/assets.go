package main

import (
	"errors"
	"html"
	"mime"
	"net/http"
	"path"
	"sefirot/internal/indexing"
	"strings"
)

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			jsonError(w, http.StatusNotFound, errors.New("unknown API route"))
			return
		}
		http.NotFound(w, r)
		return
	}
	b, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "UI unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	meta := `<meta name="vault-token" content="` + html.EscapeString(a.terminalToken) + `"><meta name="vault-path" content="` + html.EscapeString(a.vaultDir) + `">`
	b = []byte(strings.Replace(string(b), "</head>", meta+"</head>", 1))
	_, _ = w.Write(b)
}

func (a *App) handleJS(w http.ResponseWriter, r *http.Request) {
	b, err := webFiles.ReadFile("web/app.js")
	if err != nil {
		http.Error(w, "UI unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write(b)
}

func (a *App) handleStyles(w http.ResponseWriter, r *http.Request) {
	b, err := webFiles.ReadFile("web/visual-system.css")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write(b)
}

func (a *App) handleBrand(w http.ResponseWriter, r *http.Request) {
	b, err := webFiles.ReadFile("web/brand.svg")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}

func (a *App) handleTailwindCSS(w http.ResponseWriter, r *http.Request) {
	b, err := webFiles.ReadFile("web/tailwind.css")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}

func (a *App) handleVendor(w http.ResponseWriter, r *http.Request) {
	rel := path.Clean(r.PathValue("path"))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, ":") {
		http.NotFound(w, r)
		return
	}
	b, err := webFiles.ReadFile("web/vendor/" + rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(rel))
	switch strings.ToLower(path.Ext(rel)) {
	case ".js":
		contentType = "text/javascript; charset=utf-8"
	case ".css":
		contentType = "text/css; charset=utf-8"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	// Bundled third-party assets are public; sandboxed note previews have an opaque origin and need CORS to load fonts.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, _ = w.Write(b)
}

func (a *App) handleAsset(w http.ResponseWriter, r *http.Request) {
	rel, err := indexing.SafeRelativePath(r.PathValue("path"))
	if err != nil {
		http.Error(w, "invalid asset path", 400)
		return
	}
	f, err := a.root.Open(rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentTypeFor(rel))
	if strings.EqualFold(path.Ext(rel), ".svg") {
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	http.ServeContent(w, r, path.Base(rel), info.ModTime(), f)
}

func contentTypeFor(name string) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func (a *App) handleJSModule(w http.ResponseWriter, r *http.Request) {
	rel := path.Clean(r.PathValue("path"))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || !strings.HasSuffix(rel, ".js") {
		http.NotFound(w, r)
		return
	}
	b, err := webFiles.ReadFile("web/js/" + rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(b)
}
