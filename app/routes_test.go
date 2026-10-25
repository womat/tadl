package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebPageIsPublic(t *testing.T) {
	a := New(validConfig(), "", nil, nil)
	a.SetupRoutes()

	rec := httptest.NewRecorder()
	a.web.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("CSP = %q", csp)
	}
	if !strings.Contains(rec.Body.String(), "/health") && !strings.Contains(rec.Body.String(), `get("health")`) {
		t.Error("page does not read /health")
	}

	rec = httptest.NewRecorder()
	a.web.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	// {$} keeps the page to / itself. No "OPTIONS /" route (WithCORS answers
	// preflights), so an unknown path is a 404, not 405.
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("GET /unknown = %d, want 404 without the page", rec.Code)
	}
}

// A preflight needs no token: WithCORS answers it before auth and the mux.
func TestPreflightNeedsNoToken(t *testing.T) {
	a := New(validConfig(), "", nil, nil)
	a.SetupRoutes()

	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()
	a.web.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight OPTIONS /health = %d, want 204 without a token", rec.Code)
	}
}
