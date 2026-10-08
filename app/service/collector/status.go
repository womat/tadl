package collector

import (
	"time"

	"github.com/womat/golib/keyvalue"
	"github.com/womat/tadl/pkg/datalogger"
)

// trendWindow is the period the trend of a temperature covers.
const trendWindow = 15 * time.Minute

// SensorView is how the web UI shows a temperature or an output: its name and,
// for a temperature, the range of its bar in °C.
type SensorView struct {
	Label    string
	Min, Max float64
}

// TemperatureStatus is one temperature of the controller for the web UI.
type TemperatureStatus struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Value *float64 `json:"value"` // null: sensor out of range
	Min   float64  `json:"min"`
	Max   float64  `json:"max"`
	// Trend15m is the change in Kelvin over the last 15 minutes; null until
	// tadl has run that long or while the sensor is out of range.
	Trend15m *float64 `json:"trend15m"`
}

// OutputStatus is one output of the controller for the web UI.
type OutputStatus struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	On    bool   `json:"on"`
}

// DataloggerStatus is the controller part of /health.
type DataloggerStatus struct {
	Type                string              `json:"type"`
	Current             bool                `json:"current"`
	LastFrame           *string             `json:"lastFrame"`
	LastFrameAgeSeconds *float64            `json:"lastFrameAgeSeconds"`
	Temperatures        []TemperatureStatus `json:"temperatures"`
	Outputs             []OutputStatus      `json:"outputs"`
}

// sample holds the temperatures of one minute; a missing key is a sensor out of range.
type sample struct {
	at    time.Time
	temps map[string]float64
}

// track records the frame's temperatures for the trend.
// Must be called with h.mux held.
func (h *Handler) track(frame keyvalue.Record) {
	at, err := toTime(frame, datalogger.KeyTimestamp)
	if err != nil {
		return
	}
	temperatures, _ := datalogger.Keys(h.typ)

	// One sample per minute; keep one minute more than the window, so a sample
	// from at least trendWindow ago is always there once tadl has run that long.
	if n := len(h.samples); n == 0 || at.Sub(h.samples[n-1].at) >= time.Minute {
		s := sample{at: at, temps: make(map[string]float64, len(temperatures))}
		for _, key := range temperatures {
			if frame.Exists(key) {
				s.temps[key] = frame.Float64(key)
			}
		}
		h.samples = append(h.samples, s)
		for len(h.samples) > 0 && at.Sub(h.samples[0].at) > trendWindow+time.Minute {
			h.samples = h.samples[1:]
		}
	}
}

// trend returns the change of key over trendWindow up to frame time at, or nil.
// Must be called with h.mux held.
func (h *Handler) trend(key string, value float64, at time.Time) *float64 {
	// The newest sample at least trendWindow old, but not much older: after a
	// gap in the data there is no trend for the last 15 minutes.
	for i := len(h.samples) - 1; i >= 0; i-- {
		s := h.samples[i]
		age := at.Sub(s.at)
		if age < trendWindow {
			continue
		}
		if age > trendWindow+2*time.Minute {
			return nil
		}
		old, ok := s.temps[key]
		if !ok {
			return nil
		}
		d := value - old
		return &d
	}
	return nil
}

// Status returns the controller part of /health: the latest values with their
// names and ranges from views, the trend of each temperature and the state of
// each output. Without a frame the lists are empty.
func (h *Handler) Status(views map[string]SensorView) DataloggerStatus {
	h.mux.Lock()
	defer h.mux.Unlock()

	now := h.now()
	st := DataloggerStatus{
		Type:         deviceName(h.typ),
		Current:      h.isCurrent(h.DataFrame),
		Temperatures: []TemperatureStatus{},
		Outputs:      []OutputStatus{},
	}
	at, err := toTime(h.DataFrame, datalogger.KeyTimestamp)
	if err != nil {
		return st
	}
	st.LastFrame, st.LastFrameAgeSeconds = timeAndAge(at, now)

	temperatures, outputs := datalogger.Keys(h.typ)
	for _, key := range temperatures {
		v := views[key]
		t := TemperatureStatus{Key: key, Label: v.Label, Min: v.Min, Max: v.Max}
		if h.DataFrame.Exists(key) {
			value := h.DataFrame.Float64(key)
			t.Value = &value
			t.Trend15m = h.trend(key, value, at)
		}
		st.Temperatures = append(st.Temperatures, t)
	}
	for _, key := range outputs {
		st.Outputs = append(st.Outputs, OutputStatus{Key: key, Label: views[key].Label, On: h.DataFrame.Bool(key)})
	}
	return st
}

// timeAndAge formats t for JSON and returns its age at now in seconds.
func timeAndAge(t, now time.Time) (*string, *float64) {
	s := t.Format(time.RFC3339)
	age := max(now.Sub(t).Seconds(), 0)
	return &s, &age
}

func deviceName(typ int) string {
	switch typ {
	case datalogger.UVR42:
		return "uvr42"
	case datalogger.UVR31:
		return "uvr31"
	}
	return "unknown"
}
