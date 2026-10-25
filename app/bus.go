package app

// Status of the DL-Bus input for /health and the web UI.

import (
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/womat/tadl/app/service/collector"
	"github.com/womat/tadl/app/service/errwindow"
	"github.com/womat/tadl/app/service/health"
	"github.com/womat/tadl/pkg/datalogger"
)

const (
	// signalTimeout is how long frames may be missing before the signal counts as lost.
	// A UVR42 or UVR31 sends a frame about every 2.3 s.
	signalTimeout = 10 * time.Second

	// watchdogTimeout is how long edges may arrive without a frame on the bus
	// before the watchdog resets the Manchester decoder: about 13 frames of a
	// UVR42, and also the least time between two resets.
	watchdogTimeout = 30 * time.Second

	// busSampleInterval is how often the edge rate is sampled and the watchdog looks.
	busSampleInterval = 10 * time.Second
)

// decoderInfo matches decoder.Info() of github.com/womat/manchester, e.g.
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

// signalState tells from the age of the last valid record, of the last frame on
// the bus and of the last edge at the input how far data gets; a nil age means
// there was none yet. Edges without frames point at the decoder, not at the
// wiring, which is why they are a state of their own.
func signalState(recordAge, frameAge, edgeAge *time.Duration) string {
	recent := func(age *time.Duration) bool { return age != nil && *age < signalTimeout }
	switch {
	case recent(recordAge):
		return health.SignalReceiving
	case recent(frameAge):
		return health.SignalNoValidFrames
	case recent(edgeAge):
		return health.SignalNotDecoded
	default:
		return health.SignalNone
	}
}

// edgeRate returns the edges per second between two readings of the edge
// counter taken d apart.
func edgeRate(before, after uint64, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(after-before) / d.Seconds()
}

// decoderStuck tells the watchdog whether to reset the Manchester decoder:
// edges still arrive, but there was no frame on the bus for watchdogTimeout,
// counted from since (the start or the previous reset) at the latest. A silent
// line is left alone: a reset cannot help there and would only hide the fault.
func decoderStuck(now, lastEdge, lastFrame, since time.Time) bool {
	if lastEdge.IsZero() || now.Sub(lastEdge) >= signalTimeout {
		return false
	}
	if lastFrame.Before(since) {
		lastFrame = since
	}
	return now.Sub(lastFrame) > watchdogTimeout
}

// watchBus samples the edge rate for /health and resets the Manchester decoder
// when edges arrive but no frame comes out of them, until the App's context
// ends. The decoder recognises a clock locked at twice the bit rate itself;
// the watchdog is there for whatever else makes it stall.
func (app *App) watchBus() {
	ticker := time.NewTicker(busSampleInterval)
	defer ticker.Stop()

	since := time.Now()
	edges, at := app.edges.Load(), since
	// stuck is set from the first reset until a frame comes again: an input that
	// never decodes (open, mains hum, wrong bitClock) warns once, the further
	// resets go to debug and show in decoderResets.
	stuck := false
	for {
		select {
		case <-app.ctx.Done():
			return
		case now := <-ticker.C:
			n := app.edges.Load()
			rate := edgeRate(edges, n, now.Sub(at))
			app.edgeRateBits.Store(math.Float64bits(rate))
			edges, at = n, now

			lastFrame := app.dlbus.Stats().LastFrame
			if !lastFrame.Before(since) {
				stuck = false
			}
			if decoderStuck(now, app.lastEdge(), lastFrame, since) {
				level := slog.LevelWarn
				if stuck {
					level = slog.LevelDebug
				}
				slog.Log(app.ctx, level, "Edges at the DL-Bus input, but no frame: resetting the Manchester decoder",
					"edgeRateHz", math.Round(rate), "decoder", app.decoder.Info())
				stuck = true
				app.decoder.Reset()
				app.decoderResets.Add(1)
				since = now
			}
		}
	}
}

// lastEdge returns when the last edge arrived at the input, zero before the first.
func (app *App) lastEdge() time.Time {
	if ns := app.lastEdgeNs.Load(); ns != 0 {
		return time.Unix(0, ns)
	}
	return time.Time{}
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

	var recordAge, frameAge, edgeAge *time.Duration
	if dl.LastFrameAgeSeconds != nil {
		d := time.Duration(*dl.LastFrameAgeSeconds * float64(time.Second))
		recordAge = &d
	}
	if !bus.LastFrame.IsZero() {
		d := now.Sub(bus.LastFrame)
		frameAge = &d
	}
	if last := app.lastEdge(); !last.IsZero() {
		d := now.Sub(last)
		edgeAge = &d
	}

	st := health.BusStatus{
		Signal:           signalState(recordAge, frameAge, edgeAge),
		BitRateHz:        parseDecoderInfo(info),
		FixedBitClock:    app.config.DlBus.BitClock > 0,
		NominalBitRateHz: datalogger.BitRate(deviceType(app.config.DataLogger.Type)),
		EdgeRateHz:       math.Float64frombits(app.edgeRateBits.Load()),
		DecoderResets:    app.decoderResets.Load(),
		InvertedLine:     bus.InvertedLine,
		FramesReceived:   app.datalogger.Stats().Decoded,
		RejectedFrames:   counts.RejectedFrames,
		DroppedFrames:    counts.DroppedFrames,
		ProtocolErrors:   counts.ProtocolErrors,
		DroppedEdges:     counts.DroppedEdges,
		Decoder:          info,
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
