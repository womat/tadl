package app

import (
	"net/http"

	"github.com/womat/golib/web"
)

// HandleData returns the current data logger values.
//
//	@Summary		Get current data logger values
//	@Description	Returns the latest available values read from the configured data logger.
//	@Tags			data
//	@Produce		json
//	@Success		200	{object}	keyvalue.Record	"Current data logger values"
//	@Security		ApiKeyAuth
//	@Router			/data [get]
func (app *App) HandleData() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			web.Encode(w, http.StatusOK, app.dataloggerService.GetData())
		})
}
