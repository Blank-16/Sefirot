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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// terminalIdleTimeout is how long a detached session survives before it is ended.
var terminalIdleTimeout = 10 * time.Minute

const terminalReplayBytes = 256 << 10

type terminalSubscriber struct {
	ch   chan []byte
	stop chan struct{}
	conn *websocket.Conn
	once sync.Once
}

func (sub *terminalSubscriber) release() { sub.once.Do(func() { close(sub.stop) }) }

// terminalSession is owned by the server, not by a browser connection: output is
// buffered so a page reload can reattach and see what it missed.
type terminalSession struct {
	id            string
	profile       string
	startedAt     time.Time
	process       terminalProcess
	closeOnce     sync.Once
	closeErr      error
	mu            sync.Mutex
	runID         int64
	done          chan struct{}
	stopRequested bool
	replay        []byte
	sub           *terminalSubscriber
	idle          *time.Timer
	exited        bool
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
		if p.id == "wsl" && runtime.GOOS != "windows" {
			// wsl.exe is reachable through interop inside WSL, but launching it from a Linux
			// process only nests a second shell; the plain shell profile is the right one there.
			continue
		}
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
	writeJSON(w, map[string]any{
		"token": a.terminalToken, "profiles": a.terminalProfiles(),
		"platform": runtime.GOOS, "inWSL": runningInWSL(), "defaultProfile": "shell",
	})
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
		if runtime.GOOS != "windows" {
			a.terminalMu.Unlock()
			http.Error(w, "the WSL profile is only available on Windows; use the shell profile", 400)
			return
		}
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
	s := &terminalSession{id: id, profile: req.Profile, startedAt: time.Now(), process: process, runID: runID, done: make(chan struct{})}
	a.terminals[id] = s
	a.terminalMu.Unlock()
	go s.pump()
	s.armIdleTimer()
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
		s.mu.Lock()
		s.stopIdleTimer()
		s.mu.Unlock()
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

// pump moves process output into the replay buffer and to the attached browser, if any.
func (s *terminalSession) pump() {
	buf := make([]byte, 8192)
	for {
		n, err := s.process.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			s.mu.Lock()
			s.replay = append(s.replay, chunk...)
			if over := len(s.replay) - terminalReplayBytes; over > 0 {
				s.replay = append([]byte(nil), s.replay[over:]...)
			}
			sub := s.sub
			s.mu.Unlock()
			if sub != nil {
				select {
				case sub.ch <- chunk:
				case <-sub.stop:
				}
			}
		}
		if err != nil {
			s.mu.Lock()
			s.exited = true
			sub := s.sub
			s.mu.Unlock()
			if sub != nil {
				close(sub.ch)
			}
			return
		}
	}
}

func (s *terminalSession) armIdleTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idle != nil {
		s.idle.Stop()
	}
	s.idle = time.AfterFunc(terminalIdleTimeout, func() {
		s.mu.Lock()
		attached := s.sub != nil
		if !attached {
			s.stopRequested = true
		}
		s.mu.Unlock()
		if !attached {
			log.Printf("terminal %s ended after %s without a browser attached", s.id[:8], terminalIdleTimeout)
			_ = s.closeProcess()
		}
	})
}

// detachBrowser drops the attached browser connection, if any.
func (s *terminalSession) detachBrowser() {
	s.mu.Lock()
	sub := s.sub
	s.mu.Unlock()
	if sub != nil {
		sub.release()
		_ = sub.conn.Close()
	}
}

func (s *terminalSession) stopIdleTimer() {
	if s.idle != nil {
		s.idle.Stop()
	}
}

func (a *App) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	if !tokenEqual(r.URL.Query().Get("token"), a.terminalToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	a.terminalMu.Lock()
	s := a.terminals[r.URL.Query().Get("id")]
	a.terminalMu.Unlock()
	if s == nil {
		http.Error(w, "terminal session not found", http.StatusNotFound)
		return
	}
	upgrader := websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: func(r *http.Request) bool {
		// Browsers always send Origin on WebSocket handshakes. A request without one is not a
		// cross-site browser request, and the session token is still required.
		origin := r.Header.Get("Origin")
		return origin == "" || a.allowedOrigin(origin)
	}}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1 << 20)

	sub := &terminalSubscriber{ch: make(chan []byte, 256), stop: make(chan struct{}), conn: conn}
	s.mu.Lock()
	if s.exited {
		s.mu.Unlock()
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "process ended"))
		return
	}
	previous := s.sub
	s.sub = sub
	s.stopIdleTimer()
	replay := append([]byte(nil), s.replay...)
	s.mu.Unlock()
	if previous != nil {
		// A reload attaches before the old socket is noticed as closed; the newest browser wins.
		previous.release()
		_ = previous.conn.Close()
	}

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		if len(replay) > 0 {
			if conn.WriteMessage(websocket.BinaryMessage, replay) != nil {
				_ = conn.Close()
				return
			}
		}
		for {
			select {
			case chunk, ok := <-sub.ch:
				if !ok {
					_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "process ended"))
					_ = conn.Close()
					return
				}
				if conn.WriteMessage(websocket.BinaryMessage, chunk) != nil {
					_ = conn.Close()
					return
				}
			case <-sub.stop:
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
	sub.release()
	<-writerDone
	s.mu.Lock()
	detached := s.sub == sub
	if detached {
		s.sub = nil
	}
	exited := s.exited
	s.mu.Unlock()
	if detached && !exited {
		s.armIdleTimer()
	}
}

func (a *App) handleTerminalSessions(w http.ResponseWriter, r *http.Request) {
	if !a.checkTerminalToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	type item struct {
		ID        string `json:"id"`
		Profile   string `json:"profile"`
		StartedAt string `json:"startedAt"`
		Attached  bool   `json:"attached"`
	}
	out := []item{}
	a.terminalMu.Lock()
	for _, s := range a.terminals {
		s.mu.Lock()
		out = append(out, item{ID: s.id, Profile: s.profile, StartedAt: s.startedAt.UTC().Format(time.RFC3339), Attached: s.sub != nil})
		s.mu.Unlock()
	}
	a.terminalMu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt < out[j].StartedAt })
	writeJSON(w, map[string]any{"sessions": out, "idleTimeoutMinutes": int(terminalIdleTimeout.Minutes())})
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
		s.detachBrowser()
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
			s.detachBrowser()
			_ = s.closeProcess()
			<-s.done
		}(s)
	}
	wg.Wait()
}

func (a *App) checkTerminalToken(r *http.Request) bool {
	return tokenEqual(strings.TrimSpace(r.Header.Get("X-Vault-Token")), a.terminalToken)
}

// runningInWSL reports whether this Linux process runs under Windows Subsystem for Linux.
func runningInWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	raw, err := os.ReadFile("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(raw)), "microsoft")
}
