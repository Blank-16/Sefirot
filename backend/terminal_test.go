package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// startLiveServer serves the app on a real loopback port so WebSocket handshakes
// can be exercised with real Host and Origin headers.
func startLiveServer(t *testing.T) (*testApp, int) {
	t.Helper()
	a := newTestApp(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	a.port = port
	a.baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	a.h = a.routes()
	srv := &httptest.Server{Listener: l, Config: &http.Server{Handler: a.h}}
	srv.Start()
	t.Cleanup(func() { a.stopTerminals(); srv.Close() })
	return a, port
}

func (a *testApp) liveJSON(t *testing.T, method, host string, port int, path string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = strings.NewReader(string(raw))
	}
	req, _ := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", port, path), reader)
	req.Host = fmt.Sprintf("%s:%d", host, port)
	req.Header.Set("X-Vault-Token", a.terminalToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func dialTerminal(a *testApp, port int, host, origin, id string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}
	header.Set("Host", fmt.Sprintf("%s:%d", host, port))
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	return dialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/api/terminal/ws?id=%s&token=%s", port, id, a.terminalToken), header)
}

func readUntil(t *testing.T, conn *websocket.Conn, marker string) string {
	t.Helper()
	var seen strings.Builder
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			break
		}
		seen.Write(msg)
		if strings.Contains(seen.String(), marker) {
			return seen.String()
		}
	}
	t.Fatalf("did not see %q in %q", marker, seen.String())
	return ""
}

func TestTerminalProfilesHideWSLOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("profile list differs on Windows")
	}
	a := newTestApp(t)
	rec, cfg := a.do(t, "GET", "/api/terminal/config", nil)
	expect(t, rec, 200)
	for _, p := range cfg["profiles"].([]any) {
		if p.(map[string]any)["id"] == "wsl" {
			t.Fatal("the wsl profile must not be offered on a non-Windows host")
		}
	}
	if cfg["defaultProfile"] != "shell" || cfg["platform"] != runtime.GOOS {
		t.Fatalf("config = %v", cfg)
	}
	a2, port := startLiveServer(t)
	status, _ := a2.liveJSON(t, "POST", "127.0.0.1", port, "/api/terminal/start", map[string]string{"profile": "wsl"})
	if status != 400 {
		t.Fatalf("starting the wsl profile on Linux must be refused, got %d", status)
	}
}

func TestTerminalHostAndOriginHandling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY test targets Unix")
	}
	a, port := startLiveServer(t)
	status, started := a.liveJSON(t, "POST", "127.0.0.1", port, "/api/terminal/start", map[string]string{"profile": "shell"})
	if status != 200 {
		t.Fatalf("start shell: %d %v", status, started)
	}
	id := started["id"].(string)
	for _, c := range []struct {
		host, origin string
		ok           bool
	}{
		{"127.0.0.1", fmt.Sprintf("http://127.0.0.1:%d", port), true},
		{"localhost", fmt.Sprintf("http://localhost:%d", port), true},
		{"127.0.0.1", "", true},
		{"127.0.0.1", fmt.Sprintf("http://evil.example:%d", port), false},
		{"127.0.0.1", "http://localhost:1", false},
		{"evil.example", fmt.Sprintf("http://evil.example:%d", port), false},
	} {
		conn, resp, err := dialTerminal(a, port, c.host, c.origin, id)
		if c.ok && err != nil {
			t.Errorf("host=%s origin=%q should connect: %v", c.host, c.origin, err)
		}
		if !c.ok && err == nil {
			t.Errorf("host=%s origin=%q must be rejected", c.host, c.origin)
		}
		if conn != nil {
			conn.Close()
		}
		_ = resp
	}
}

func TestTerminalSessionSurvivesDisconnectAndReplays(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY test targets Unix")
	}
	a, port := startLiveServer(t)
	_, started := a.liveJSON(t, "POST", "127.0.0.1", port, "/api/terminal/start", map[string]string{"profile": "shell"})
	id, _ := started["id"].(string)
	if id == "" {
		t.Fatalf("no session: %v", started)
	}
	origin := fmt.Sprintf("http://127.0.0.1:%d", port)
	conn, _, err := dialTerminal(a, port, "127.0.0.1", origin, id)
	if err != nil {
		t.Fatal(err)
	}
	conn.WriteMessage(websocket.BinaryMessage, []byte("echo MARK$((20+22))\n"))
	readUntil(t, conn, "MARK42")
	conn.Close()
	time.Sleep(300 * time.Millisecond)

	_, list := a.liveJSON(t, "GET", "127.0.0.1", port, "/api/terminal/sessions", nil)
	sessions := list["sessions"].([]any)
	if len(sessions) != 1 || sessions[0].(map[string]any)["attached"] != false {
		t.Fatalf("a closed browser must leave the session running and detached: %v", list)
	}

	again, _, err := dialTerminal(a, port, "localhost", fmt.Sprintf("http://localhost:%d", port), id)
	if err != nil {
		t.Fatalf("reattach failed: %v", err)
	}
	defer again.Close()
	readUntil(t, again, "MARK42")
	again.WriteMessage(websocket.BinaryMessage, []byte("echo SECOND$((1+1))\n"))
	readUntil(t, again, "SECOND2")

	takeover, _, err := dialTerminal(a, port, "127.0.0.1", origin, id)
	if err != nil {
		t.Fatalf("takeover by a reloaded page failed: %v", err)
	}
	defer takeover.Close()
	readUntil(t, takeover, "SECOND2")

	a.liveJSON(t, "POST", "127.0.0.1", port, "/api/terminal/stop", map[string]string{"id": id})
	_, list = a.liveJSON(t, "GET", "127.0.0.1", port, "/api/terminal/sessions", nil)
	if len(list["sessions"].([]any)) != 0 {
		t.Fatalf("explicit stop must end the session: %v", list)
	}
}

func TestDetachedTerminalIsEndedAfterIdleTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY test targets Unix")
	}
	old := terminalIdleTimeout
	terminalIdleTimeout = 400 * time.Millisecond
	defer func() { terminalIdleTimeout = old }()
	a, port := startLiveServer(t)
	_, started := a.liveJSON(t, "POST", "127.0.0.1", port, "/api/terminal/start", map[string]string{"profile": "shell"})
	id := started["id"].(string)
	conn, _, err := dialTerminal(a, port, "127.0.0.1", "", id)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		_, list := a.liveJSON(t, "GET", "127.0.0.1", port, "/api/terminal/sessions", nil)
		if len(list["sessions"].([]any)) == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("an unattended session must be ended after the idle timeout")
}
