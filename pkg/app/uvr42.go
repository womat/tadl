package app

import (
	"github.com/womat/debug"
	"time"
)

func (app *App) readUVR42() {
	for {
		if d, err := app.uvr42.Get(); err != nil {
			debug.ErrorLog.Println(err)
			time.Sleep(time.Second)
		} else {
			app.uvr42Data = d
		}
	}
}
