package app

import (
	"tadl/pkg/raspberry"
	"time"
)

// testPinEmu emulate ticks on gpio pin, only for testing in windows mode
func testPinEmu(p raspberry.Pin) {
	for range time.Tick(time.Duration(p.Pin()/2) * time.Second) {
		p.EmuEdge(raspberry.EdgeFalling)
	}
}

func (app *App) handler(p raspberry.Pin) {
	// t := time.Now()

	// find the measuring device based on the pin configuration
	if app.config.Gpio == p.Pin() {
		app.dl.Sync()
	}

	// debug.DebugLog.Printf("runtime Sync (call): %v", time.Now().Sub(t))
}
