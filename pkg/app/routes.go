package app

// initDefaultRoutes initializes the applications default routes.
//  These are the routes which always are the same in every application.
//  Things like user api, version, ...
func (app *App) initDefaultRoutes() {
	api := app.web.Group("/")
	if app.config.Webserver.Webservices["version"] {
		api.Get("/version", app.HandleVersion())
	}
	if app.config.Webserver.Webservices["health"] {
		api.Get("/health", app.HandleHealth())
	}
	if app.config.Webserver.Webservices["data"] {
		api.Get("/data", app.HandleData())
	}
}
