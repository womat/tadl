// Package health provides basic system and application health information.
//
// It collects metrics such as memory usage, goroutine count, uptime, host info,
// Go runtime version, and application version. The module is intended to be
// lightweight and easily serializable to JSON for monitoring or diagnostic purposes.
package health

import (
	"os"
	"runtime"
	"time"

	"github.com/womat/tadl/app/service/collector"
	"github.com/womat/tadl/app/service/errwindow"
)

// MQTT connection states reported in Model.Mqtt.
const (
	MqttConnected    = "connected"
	MqttDisconnected = "disconnected"
	MqttDisabled     = "disabled"
)

// DL-Bus signal states reported in BusStatus.Signal.
const (
	SignalReceiving     = "receiving"     // valid frames arrive
	SignalNoValidFrames = "noValidFrames" // frames arrive, but none fits the configured controller
	SignalNone          = "noSignal"      // no frames on the bus
)

// BusStatus describes the DL-Bus input.
type BusStatus struct {
	Signal       string   `json:"signal"`
	BitRateHz    *float64 `json:"bitRateHz"` // recovered bit clock; null while the decoder searches for it
	InvertedLine bool     `json:"invertedLine"`

	// Cumulative counters since start.
	FramesReceived uint64 `json:"framesReceived"`
	RejectedFrames uint64 `json:"rejectedFrames"`
	DroppedFrames  uint64 `json:"droppedFrames"`
	ProtocolErrors uint64 `json:"protocolErrors"`
	DroppedEdges   uint64 `json:"droppedEdges"`

	Errors24h           ErrorSummary `json:"errors24h"`
	LastError           *string      `json:"lastError"` // last error within the last 24 h; null if none
	LastErrorAgeSeconds *float64     `json:"lastErrorAgeSeconds"`

	Decoder string `json:"decoder"` // decoder state as text, for troubleshooting
}

// ErrorSummary is the number of errors per kind and in total.
type ErrorSummary struct {
	errwindow.Counts
	Total uint64 `json:"total"`
}

// Model holds the main system and runtime health information.
type Model struct {
	App            string  `json:"app"`            // Application name (MODULE)
	AppVersion     string  `json:"appVersion"`     // Current version of the application
	GoVersion      string  `json:"goVersion"`      // Go runtime version
	Hostname       string  `json:"hostname"`       // Machine name where the app runs
	OS             string  `json:"os"`             // Operating system name
	UptimeSeconds  float64 `json:"uptimeSeconds"`  // Application uptime in seconds
	NumGoroutines  int     `json:"numGoroutines"`  // Current number of active goroutines
	HeapAllocBytes uint64  `json:"heapAllocBytes"` // Allocated heap memory in bytes
	SysMemoryBytes uint64  `json:"sysMemoryBytes"` // Total memory obtained from the OS
	Timestamp      string  `json:"timestamp"`      // UTC timestamp when health info was collected (RFC3339)

	Mqtt       string                      `json:"mqtt"`                 // connected | disconnected | disabled
	MqttBroker string                      `json:"mqttBroker,omitempty"` // broker host:port, never user or password; absent without MQTT
	MqttTopic  string                      `json:"mqttTopic,omitempty"`  // topic the values are published to; absent without MQTT
	Datalogger *collector.DataloggerStatus `json:"datalogger"`           // latest values of the controller
	Bus        *BusStatus                  `json:"bus"`                  // state of the DL-Bus input
}

var startTime = time.Now() // Tracks application start time

// GetCurrentHealth returns the current system and application health data.
func GetCurrentHealth(module, version string) Model {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return Model{
		App:            module,
		AppVersion:     version,
		GoVersion:      runtime.Version(),
		Hostname:       host,
		OS:             runtime.GOOS,
		UptimeSeconds:  time.Since(startTime).Seconds(),
		NumGoroutines:  runtime.NumGoroutine(),
		HeapAllocBytes: mem.Alloc,
		SysMemoryBytes: mem.Sys,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}
}
