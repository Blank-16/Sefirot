package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type terminalSession struct {
	id            string
	process       terminalProcess
	closeOnce     sync.Once
	closeErr      error
	mu            sync.Mutex
	runID         int64
	done          chan struct{}
	stopRequested bool
	connected     bool
}

func (s *terminalSession) closeProcess() error {
	s.closeOnce.Do(func() { s.closeErr = s.process.Close() })
	return s.closeErr
}

type terminalStartRequest struct {
	Profile string `json:"profile"`
}
type terminalStartResponse struct {
	ID string `json:"id"`
}

func tokenEqual(got, expected string) bool {
	if len(got) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func setEnvironmentValue(env []string, key, value string) []string {
	prefix := key + "="
	for i, item := range env {
		matches := strings.HasPrefix(item, prefix)
		if runtime.GOOS == "windows" && len(item) >= len(prefix) {
			matches = strings.EqualFold(item[:len(prefix)], prefix)
		}
		if matches {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func (a *App) terminalProfiles() []map[string]string {
	profiles := []map[string]string{}
	for _, p := range []struct{ id, label, bin string }{{"shell", "Shell", ""}, {"wsl", "WSL", "wsl.exe"}, {"claude", "Claude Code", "claude"}, {"codex", "Codex CLI", "codex"}, {"gemini", "Gemini CLI", "gemini"}, {"aider", "Aider", "aider"}, {"agy", "AGY (custom executable)", "agy"}} {
		if !terminalBackendAvailable() {
			profiles = append(profiles, map[string]string{"id": p.id, "label": p.label, "available": "false"})
			continue
		}
		if p.id == "shell" {
			profiles = append(profiles, map[string]string{"id": p.id, "label": p.label, "available": "true"})
			continue
		}
		_, err := exec.LookPath(p.bin)
		available := "false"
		if err == nil {
			available = "true"
		}
		profiles = append(profiles, map[string]string{"id": p.id, "label": p.label, "available": available})
	}
	return profiles
}

func (a *App) handleTerminalConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"token": a.terminalToken, "profiles": a.terminalProfiles()})
}

func (a *App) handleTerminalStart(w http.ResponseWriter, r *http.Request) {
	if !tokenEqual(r.Header.Get("X-Vault-Token"), a.terminalToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req terminalStartRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		jsonError(w, 400, err)
		return
	}
	a.terminalMu.Lock()
	if len(a.terminals) >= 4 {
		a.terminalMu.Unlock()
		http.Error(w, "terminal session limit reached (4)", http.StatusTooManyRequests)
		return
	}
	var cmd *exec.Cmd
	switch req.Profile {
	case "shell":
		cmd = defaultShellCommand()
	case "wsl":
		cmd = exec.Command("wsl.exe")
	case "claude":
		cmd = exec.Command("claude")
	case "codex":
		cmd = exec.Command("codex")
	case "gemini":
		cmd = exec.Command("gemini")
	case "aider":
		cmd = exec.Command("aider")
	case "agy":
		cmd = exec.Command("agy")
	default:
		a.terminalMu.Unlock()
		http.Error(w, "unknown terminal profile", 400)
		return
	}
	bin, err := exec.LookPath(cmd.Path)
	if err != nil {
		a.terminalMu.Unlock()
		http.Error(w, fmt.Sprintf("%s is not installed or not on PATH", cmd.Path), 404)
		return
	}
	cmd.Path = bin
	if runtime.GOOS == "windows" {
		switch strings.ToLower(filepath.Ext(bin)) {
		case ".cmd", ".bat":
			// npm-installed CLI tools commonly resolve to .cmd shims; CreateProcess
			// cannot launch batch files directly, so route them through cmd.exe.
			cmd = exec.Command("cmd.exe", "/d", "/c", `"`+bin+`"`)
		}
	}
	cwd, err := filepath.Abs(a.vaultDir)
	if err != nil {
		a.terminalMu.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}
	cmd.Dir = cwd
	cmd.Env = setEnvironmentValue(os.Environ(), "TERM", "xterm-256color")
	cmd.Env = setEnvironmentValue(cmd.Env, "COLORTERM", "truecolor")
	cmd.Env = setEnvironmentValue(cmd.Env, "SEFIROT_URL", a.baseURL)
	process, err := startTerminalProcess(cmd)
	if err != nil {
		a.terminalMu.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}
	id, err := randomToken()
	if err != nil {
		_ = process.Close()
		_, _ = process.Wait()
		a.terminalMu.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}
	runID, runErr := a.db.StartAgentRun(context.Background(), req.Profile, cwd)
	if runErr != nil {
		_ = process.Close()
		_, _ = process.Wait()
		a.terminalMu.Unlock()
		http.Error(w, runErr.Error(), 500)
		return
	}
	s := &terminalSession{id: id, process: process, runID: runID, done: make(chan struct{})}
	a.terminals[id] = s
	a.terminalMu.Unlock()
	go func() {
		exitCode, waitErr := process.Wait()
		s.mu.Lock()
		stopped := s.stopRequested
		s.mu.Unlock()
		status := "completed"
		if stopped {
			status = "stopped"
		} else if waitErr != nil {
			status = "failed"
		}
		_ = a.db.FinishAgentRun(context.Background(), runID, status, exitCode)
		if closeErr := s.closeProcess(); closeErr != nil {
			log.Printf("terminal %s cleanup: %v", id, closeErr)
		}
		a.terminalMu.Lock()
		if a.terminals[id] == s {
			delete(a.terminals, id)
		}
		a.terminalMu.Unlock()
		close(s.done)
	}()
	writeJSON(w, terminalStartResponse{ID: id})
}

func (a *App) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	if !tokenEqual(r.URL.Query().Get("token"), a.terminalToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := r.URL.Query().Get("id")
	a.terminalMu.Lock()
	s := a.terminals[id]
	a.terminalMu.Unlock()
	if s == nil {
		http.Error(w, "terminal session not found", http.StatusNotFound)
		return
	}
	s.mu.Lock()
	if s.connected {
		s.mu.Unlock()
		http.Error(w, "terminal session already connected", http.StatusConflict)
		return
	}
	s.connected = true
	s.mu.Unlock()
	upgrader := websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: func(r *http.Request) bool { return a.allowedOrigin(r.Header.Get("Origin")) }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.mu.Lock()
		s.connected = false
		s.mu.Unlock()
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1 << 20)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 8192)
		for {
			n, e := s.process.Read(buf)
			if n > 0 {
				if writeErr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					_ = conn.Close()
					return
				}
			}
			if e != nil {
				_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "process ended"))
				_ = conn.Close()
				return
			}
		}
	}()
	for {
		_, msg, e := conn.ReadMessage()
		if e != nil {
			break
		}
		// JSON control messages resize the PTY; all other frames are terminal input.
		var control struct {
			Type string `json:"type"`
			Rows uint16 `json:"rows"`
			Cols uint16 `json:"cols"`
		}
		if json.Unmarshal(msg, &control) == nil && control.Type == "resize" {
			_ = s.process.Resize(control.Rows, control.Cols)
			continue
		}
		if _, e = s.process.Write(msg); e != nil {
			break
		}
	}
	_ = conn.Close()
	s.mu.Lock()
	s.stopRequested = true
	s.connected = false
	s.mu.Unlock()
	_ = s.closeProcess()
	<-done
	<-s.done
}

func (a *App) handleTerminalStop(w http.ResponseWriter, r *http.Request) {
	if !tokenEqual(r.Header.Get("X-Vault-Token"), a.terminalToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req terminalStartResponse
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		jsonError(w, 400, err)
		return
	}
	a.terminalMu.Lock()
	s := a.terminals[req.ID]
	delete(a.terminals, req.ID)
	a.terminalMu.Unlock()
	if s != nil {
		s.mu.Lock()
		s.stopRequested = true
		s.mu.Unlock()
		_ = s.closeProcess()
		<-s.done
	}
	writeJSON(w, map[string]bool{"stopped": true})
}

func (a *App) stopTerminals() {
	a.terminalMu.Lock()
	sessions := make([]*terminalSession, 0, len(a.terminals))
	for id, s := range a.terminals {
		s.mu.Lock()
		s.stopRequested = true
		s.mu.Unlock()
		sessions = append(sessions, s)
		delete(a.terminals, id)
	}
	a.terminalMu.Unlock()
	var wg sync.WaitGroup
	for _, s := range sessions {
		wg.Add(1)
		go func(s *terminalSession) {
			defer wg.Done()
			_ = s.closeProcess()
			<-s.done
		}(s)
	}
	wg.Wait()
}

func (a *App) checkTerminalToken(r *http.Request) bool {
	return tokenEqual(strings.TrimSpace(r.Header.Get("X-Vault-Token")), a.terminalToken)
}
