package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const maxSettingsBytes = 64 << 10

// Settings holds interface preferences. They are stored beside the index, not in
// the browser, so they survive origin changes (desktop port selection, WSL/Windows).
type Settings struct {
	Theme        string         `json:"theme"`
	Tabs         []string       `json:"tabs"`
	ResearchDocs []string       `json:"researchDocs"`
	Macros       map[string]any `json:"macros"`
	DOILookup    bool           `json:"doiLookup"`
}

type settingsStore struct {
	mu   sync.Mutex
	path string
}

func defaultSettings() Settings {
	return Settings{Theme: "dark", Tabs: []string{}, ResearchDocs: []string{}, Macros: map[string]any{}}
}

func validateSettings(s *Settings) error {
	if s.Theme != "dark" && s.Theme != "light" {
		return errors.New("theme must be dark or light")
	}
	if s.Tabs == nil {
		s.Tabs = []string{}
	}
	if s.ResearchDocs == nil {
		s.ResearchDocs = []string{}
	}
	if s.Macros == nil {
		s.Macros = map[string]any{}
	}
	if len(s.Tabs) > 50 || len(s.ResearchDocs) > 5000 {
		return errors.New("too many tabs or research documents")
	}
	for _, p := range append(append([]string{}, s.Tabs...), s.ResearchDocs...) {
		if len(p) == 0 || len(p) > 1024 {
			return errors.New("invalid document path")
		}
	}
	if len(s.Macros) > 50 {
		return errors.New("a maximum of 50 macros is supported")
	}
	for name, def := range s.Macros {
		if name == "" || len(name) > 32 {
			return errors.New("invalid macro name")
		}
		switch v := def.(type) {
		case string:
			if len(v) == 0 || len(v) > 300 {
				return fmt.Errorf("macro %s: definition must be 1-300 characters", name)
			}
		case []any:
			text, ok := "", false
			if len(v) == 2 {
				text, ok = v[0].(string)
			}
			n, nok := float64(0), false
			if len(v) == 2 {
				n, nok = v[1].(float64)
			}
			if !ok || !nok || len(text) == 0 || len(text) > 300 || n < 0 || n > 9 || n != float64(int(n)) {
				return fmt.Errorf("macro %s: expected [definition, arguments 0-9]", name)
			}
		default:
			return fmt.Errorf("macro %s: unsupported definition", name)
		}
	}
	return nil
}

func (st *settingsStore) load() (Settings, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	raw, err := os.ReadFile(st.path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultSettings(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	s := defaultSettings()
	if err := json.Unmarshal(raw, &s); err != nil || validateSettings(&s) != nil {
		return defaultSettings(), nil
	}
	return s, nil
}

func (st *settingsStore) save(s Settings) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(st.path), "settings-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(raw)
	if werr == nil {
		werr = tmp.Sync()
	}
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(tmp.Name())
		return werr
	}
	if err := os.Rename(tmp.Name(), st.path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

func (a *App) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	s, err := a.settings.load()
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, s)
}

func (a *App) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSettingsBytes)
	var s Settings
	if err := decodeJSON(r, &s); err != nil {
		jsonError(w, 400, err)
		return
	}
	if err := validateSettings(&s); err != nil {
		jsonError(w, 400, err)
		return
	}
	if err := a.settings.save(s); err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, s)
}

func (a *App) handleIntegrity(w http.ResponseWriter, r *http.Request) {
	rep, err := a.db.Integrity(r.Context())
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	writeJSON(w, rep)
}

func (a *App) handleResolve(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(r.URL.Query().Get("target"))
	if target == "" || len(target) > 512 {
		jsonError(w, 400, errors.New("target is required"))
		return
	}
	idx, err := a.db.LinkIndex(r.Context())
	if err != nil {
		jsonError(w, 500, err)
		return
	}
	resolved, candidates := idx.Resolve(target)
	if candidates == nil {
		candidates = []string{}
	}
	writeJSON(w, map[string]any{"path": resolved, "candidates": candidates})
}
