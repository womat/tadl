package app

import (
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/womat/debug"
)

// HandleHealth returns data about the health of myself.
// output example:
//  {"JobCount":2,"NumGoroutines":11,"HeapAllocatedBytes":332256360,"HeapAllocatedMB":316,
//   "SysMemoryBytes":360290312,"SysMemoryMB":343,"Version":"0.0.0+20200516","ProgLang":"go1.15.2"}
func (app *App) HandleHealth() fiber.Handler {
	bToMb := func(b uint64) uint64 {
		return b / 1024 / 1024
	}

	host, _ := os.Hostname()

	return func(ctx *fiber.Ctx) error {
		debug.InfoLog.Print("web request health")

		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		hab := m.Alloc
		smb := m.Sys

		healthData := struct {
			NumGoroutines      int
			NumCPU             int
			HeapAllocatedBytes uint64
			HeapAllocatedMB    uint64
			SysMemoryBytes     uint64
			SysMemoryMB        uint64
			Version            string
			ProgLang           string
			HostName           string
			Time               string
		}{
			NumGoroutines:      runtime.NumGoroutine(),
			NumCPU:             runtime.NumCPU(),
			HeapAllocatedBytes: hab,
			HeapAllocatedMB:    bToMb(hab),
			SysMemoryBytes:     smb,
			SysMemoryMB:        bToMb(smb),
			ProgLang:           runtime.Version(),
			Version:            VERSION,
			HostName:           host,
			Time:               time.Now().Format(time.RFC3339),
		}
		ctx.Status(http.StatusOK)
		return ctx.JSON(healthData)
	}
}
