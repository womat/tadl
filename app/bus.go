package app

// Status of the DL-Bus input for /health and the web UI.

import (
	"regexp"
	"strconv"
	"time"

	"github.com/womat/tadl/app/service/collector"
	"github.com/womat/tadl/app/service/errwindow"
	"github.com/womat/tadl/app/service/health"
	"github.com/womat/tadl/pkg/datalogger"
)

// signalTimeout is how long frames may be missing before the signal counts as lost.
// A UVR42 or UVR31 sends a frame about every 2.3 s.
const signalTimeout = 10 * time.Second

// decoderInfo matches golib's decoder.Info(), e.g.
// "Decoder state: decoding data, Frequency: 50.00 Hz, Buffer overflow count: 0, Resync count: 3".
var decoderInfo = regexp.MustCompile(`Decoder state: ([^,]+), Frequency: ([0-9.]+) Hz`)

// parseDecoderInfo returns the bit rate the decoder follows, or nil while it
// searches for the clock or when the text has an unknown format.
func parseDecoderInfo(info string) *float64 {
	m := decoderInfo.FindStringSubmatch(info)
	if m == nil || m[1] != "decoding data" {
		return nil
	}
	hz, err := strconv.ParseFloat(m[2], 64)
	if err != nil || hz <= 0 {
		return nil
	}
	return &hz
}

// signalState tells from the age of the last valid record and of the last frame
// on the bus whether data arrives; a nil age means there was none yet.
func signalState(recordAge, frameAge *time.Duration) string {
	switch {
	case recordAge != nil && *recordAge < signalTimeout:
		return health.SignalReceiving
	case frameAge != nil && *frameAge < signalTimeout:
		return health.SignalNoValidFrames
	default:
		return health.SignalNone
	}
}

// busErrorCounts returns the cumulative error counters of the DL-Bus input.
func (app *App) busErrorCounts() errwindow.Counts {
	bus := app.dlbus.Stats()
	return errwindow.Counts{
		RejectedFrames: app.datalogger.Stats().Rejected,
		DroppedFrames:  bus.DroppedFrames,
		ProtocolErrors: bus.ProtocolErrors,
		DroppedEdges:   app.pin.DroppedEvents() + app.droppedEdges.Load(),
	}
}

// busStatus returns the state of the DL-Bus input. dl is the controller status
// of the same /health response, for the age of the last valid record.
func (app *App) busStatus(dl collector.DataloggerStatus, now time.Time) health.BusStatus {
	bus := app.dlbus.Stats()
	counts := app.busErrorCounts()
	info := app.decoder.Info()

	var recordAge, frameAge *time.Duration
	if dl.LastFrameAgeSeconds != nil {
		d := time.Duration(*dl.LastFrameAgeSeconds * float64(time.Second))
		recordAge = &d
	}
	if !bus.LastFrame.IsZero() {
		d := now.Sub(bus.LastFrame)
		frameAge = &d
	}

	st := health.BusStatus{
		Signal:         signalState(recordAge, frameAge),
		BitRateHz:      parseDecoderInfo(info),
		InvertedLine:   bus.InvertedLine,
		FramesReceived: app.datalogger.Stats().Decoded,
		RejectedFrames: counts.RejectedFrames,
		DroppedFrames:  counts.DroppedFrames,
		ProtocolErrors: counts.ProtocolErrors,
		DroppedEdges:   counts.DroppedEdges,
		Decoder:        info,
	}

	last24h, lastError := app.busErrors.Last24h()
	st.Errors24h = health.ErrorSummary{Counts: last24h, Total: last24h.Total()}
	if !lastError.IsZero() {
		s := lastError.Format(time.RFC3339)
		age := max(now.Sub(lastError).Seconds(), 0)
		st.LastError, st.LastErrorAgeSeconds = &s, &age
	}
	return st
}

// sensorViews returns the name and bar range of every temperature and output of
// the configured controller, with the defaults filled in.
func (app *App) sensorViews() map[string]collector.SensorView {
	temperatures, outputs := datalogger.Keys(deviceType(app.config.DataLogger.Type))
	views := make(map[string]collector.SensorView, len(temperatures)+len(outputs))
	for _, key := range temperatures {
		s := app.config.DataLogger.Sensors[key]
		lo, hi := s.SensorRange()
		views[key] = collector.SensorView{Label: s.Label, Min: lo, Max: hi}
	}
	for _, key := range outputs {
		views[key] = collector.SensorView{Label: app.config.DataLogger.Sensors[key].Label}
	}
	return views
}
