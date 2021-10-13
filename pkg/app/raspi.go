package app

import (
	"tadl/pkg/raspberry"
	"time"

	"github.com/womat/debug"
)

// testPinEmu emulate ticks on gpio pin, only for testing in windows mode
func testPinEmu(p raspberry.Pin) {
	for range time.Tick(time.Duration(p.Pin()/2) * time.Second) {
		p.EmuEdge(raspberry.EdgeFalling)
	}
}

func (app *App) handler(p raspberry.Pin) {
	pin := p.Pin()
	// find the measuring device based on the pin configuration
	if app.config.Gpio == pin {
		// app.lineHandler.Unwatch()
		go app.dl.Restart()
		return
	} else {
		debug.TraceLog.Printf("receive a negative impulse from wrong pin: %v", pin)
	}
}
