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
	// {$} keeps the page to / itself. The router answers 405 here, not 404,
	// because the preflight route OPTIONS / matches every path.
	if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("GET /unknown = %d and serves the page", rec.Code)
	}
}
