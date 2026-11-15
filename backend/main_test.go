package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalOnlyUsesSelectedPort(t *testing.T) {
	called := false
	h := localOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}), 43127)

	allowed := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43127/", nil)
	allowed.Host = "127.0.0.1:43127"
	response := httptest.NewRecorder()
	h.ServeHTTP(response, allowed)
	if response.Code != http.StatusNoContent || !called {
		t.Fatalf("expected selected loopback port to pass, status=%d", response.Code)
	}

	called = false
	blocked := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/", nil)
	blocked.Host = "127.0.0.1:8080"
	response = httptest.NewRecorder()
	h.ServeHTTP(response, blocked)
	if response.Code != http.StatusForbidden || called {
		t.Fatalf("expected mismatched port to be blocked, status=%d", response.Code)
	}
}

func TestAllowedOriginMatchesSelectedLoopbackPort(t *testing.T) {
	a := &App{port: 43127}
	for _, origin := range []string{"http://127.0.0.1:43127", "http://localhost:43127"} {
		if !a.allowedOrigin(origin) {
			t.Errorf("expected origin to be allowed: %s", origin)
		}
	}
	for _, origin := range []string{"http://127.0.0.1:8080", "https://127.0.0.1:43127", "http://example.com:43127", "http://127.0.0.1:43127/evil"} {
		if a.allowedOrigin(origin) {
			t.Errorf("expected origin to be rejected: %s", origin)
		}
	}
}

func TestShutdownRequiresPerProcessToken(t *testing.T) {
	a := &App{terminalToken: "process-secret", shutdown: make(chan struct{})}
	h := http.HandlerFunc(a.handleShutdown)

	unauthorized := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43127/api/shutdown", nil)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, unauthorized)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized response, got %d", response.Code)
	}
	select {
	case <-a.shutdown:
		t.Fatal("unauthorized request triggered shutdown")
	default:
	}

	authorized := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43127/api/shutdown", nil)
	authorized.Header.Set("X-Vault-Token", "process-secret")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, authorized)
	if response.Code != http.StatusOK {
		t.Fatalf("expected successful shutdown response, got %d", response.Code)
	}
	select {
	case <-a.shutdown:
	default:
		t.Fatal("authorized request did not trigger shutdown")
	}
}

func TestHandleJSModuleServesBundledFeatureModule(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/js/core.js", nil)
	req.SetPathValue("path", "core.js")
	res := httptest.NewRecorder()
	(&App{}).handleJSModule(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected module response, got %d", res.Code)
	}
	if got := res.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", got)
	}
	if !strings.Contains(res.Body.String(), "const state") {
		t.Fatal("expected the shared frontend state module")
	}
}
