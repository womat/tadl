// Package app sets up HTTP routes and middleware for the application.
// It supports authentication, Swagger documentation (dev only), and monitoring endpoints.
// Routes:
// - Public routes without authentication: the web page at /, version
// - Protected routes requiring a bearer token from the token file
// - Swagger documentation (only in development) at /swagger/
// - Health, Version, and data endpoints
//
// Middleware applied:
// - CORS
// - IP filtering (allowed/blocked IPs)
//
// This must be called during app startup before starting the HTTP server.
package app

import (
	"log/slog"
	"net/http"

	"github.com/womat/golib/v2/web"
)

// SetupRoutes configures all HTTP routes and global middleware for the application.
func (app *App) SetupRoutes() {
	webCfg := app.webConfig()

	mux := http.NewServeMux()

	// Dev-only Swagger documentation (only registered with -tags swagger)
	app.registerSwaggerRoute(mux)

	// Public routes. {$} matches / only, so the page does not catch unknown paths.
	mux.Handle("GET /{$}", app.HandleUI())
	mux.Handle("GET /version", app.HandleVersion())

	// Protected routes
	mux.Handle("GET /health", web.WithAuth(app.HandleHealth(), webCfg))
	mux.Handle("GET /data", web.WithAuth(app.HandleData(), webCfg))

	// Apply global middleware: CORS + IP filter. WithCORS answers a preflight itself, before
	// auth; an "OPTIONS /" route is not needed and would turn the mux's 404 for an unknown path
	// into 405.
	handler := web.WithCORS(mux)
	handler = web.WithIPFilter(handler, app.config.Webserver.AllowedIPs, app.config.Webserver.BlockedIPs)
	// One entry per request with client and caller. The web page polls every few seconds, so
	// successful requests go to Debug; a rejected one shows as a warning.
	handler = web.WithLogging(handler, slog.Default(), web.WithSuccessLevel(slog.LevelDebug))
	app.web.Handler = handler
}

// webConfig is the authentication of the protected routes: the bearer tokens from the token
// file, nothing else.
func (app *App) webConfig() web.Config {
	return web.Config{Tokens: app.config.Webserver.Tokens}
}
