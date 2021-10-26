package app

import (
	"github.com/gofiber/fiber/v2"
	"github.com/womat/debug"
)

// runWebServer starts the applications web server and listens for web requests.
//  It's designed to run in a separate go function to not block the main go function.
//  e.g.: go runWebServer()
//  See app.Run()
func (app *App) runWebServer() {
	err := app.web.Listen(app.urlParsed.Host)
	debug.ErrorLog.Print(err)
}

// HandleData returns the data frame of the controller.
func (app *App) HandleData() fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		debug.InfoLog.Print("web request data")

		app.DataFrame.Lock()
		defer app.DataFrame.Unlock()
		return ctx.JSON(app.DataFrame.data)
	}
}
