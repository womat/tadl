package app

import (
	"net/http"

	"github.com/womat/golib/web"
)

// HandleData returns the current data logger values.
//
//	@Summary		Get current data logger values
//	@Description	Returns the latest available values read from the configured data logger.
//	@Description	The response structure depends on the configured device type:
//	@Description	UVR42 returns temperature1–4, out1, out2 and a timestamp.
//	@Description	UVR31 returns temperature1–3, out1 and a timestamp.
//	@Tags			data
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Success		200	{object}	object{timestamp=string,temperature1=number,temperature2=number,temperature3=number,temperature4=number,out1=boolean,out2=boolean}	"Current data logger values"
//	@Failure		401	{string}	string																																"Unauthorized"
//	@Router			/data [get]
func (app *App) HandleData() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			web.Encode(w, http.StatusOK, app.dataloggerService.GetData())
		})
}
