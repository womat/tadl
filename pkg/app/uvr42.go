package app

import (
	"github.com/womat/debug"
	"io"
	"time"
)

func (app *App) readUVR42() {
	for {
		if d, err := app.uvr42.Get(); err != nil {
			if err == io.EOF {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			debug.ErrorLog.Println(err)
		} else {
			app.uvr42Data = d
			debug.InfoLog.Printf("UVR42: %v", d)
		}
	}
}
