package collector

import (
	"testing"
	"time"

	"github.com/womat/golib/keyvalue"
	"github.com/womat/tadl/pkg/datalogger"
)

var views = map[string]SensorView{
	datalogger.KeyTemperature1: {Label: "Collector", Min: -20, Max: 150},
	datalogger.KeyTemperature2: {Min: -20, Max: 150},
	datalogger.KeyTemperature3: {Min: -20, Max: 150},
	datalogger.KeyTemperature4: {Label: "Boiler room", Min: -5, Max: 25},
	datalogger.KeyOut1:         {Label: "Solar pump"},
}

func TestStatusWithoutFrame(t *testing.T) {
	st := newHandler(0.5).Status(views)
	if st.Type != "uvr42" || st.Current || st.LastFrame != nil || len(st.Temperatures) != 0 || len(st.Outputs) != 0 {
		t.Errorf("status without frame = %+v", st)
	}
}

func TestStatusValuesNamesAndRanges(t *testing.T) {
	h := newHandler(0.5)
	f := uvr42Frame(t0, 64.3, true)
	delete(f, datalogger.KeyTemperature3) // sensor out of range
	h.checkAndUpdate(f)
	h.now = func() time.Time { return t0.Add(2 * time.Second) }

	st := h.Status(views)
	if !st.Current || st.LastFrameAgeSeconds == nil || *st.LastFrameAgeSeconds != 2 {
		t.Errorf("current=%v age=%v, want current and 2 s", st.Current, st.LastFrameAgeSeconds)
	}
	if len(st.Temperatures) != 4 || len(st.Outputs) != 2 {
		t.Fatalf("%d temperatures, %d outputs, want 4 and 2", len(st.Temperatures), len(st.Outputs))
	}
	t1, t3, t4 := st.Temperatures[0], st.Temperatures[2], st.Temperatures[3]
	if t1.Label != "Collector" || *t1.Value != 64.3 || t1.Min != -20 || t1.Max != 150 {
		t.Errorf("temperature1 = %+v", t1)
	}
	if t3.Value != nil {
		t.Errorf("temperature3 out of range: value = %v, want null", *t3.Value)
	}
	if t4.Min != -5 || t4.Max != 25 {
		t.Errorf("temperature4 range = %v…%v", t4.Min, t4.Max)
	}
	if o := st.Outputs[0]; o.Label != "Solar pump" || !o.On || st.Outputs[1].On {
		t.Errorf("outputs = %+v", st.Outputs)
	}
}

// frameAt returns a UVR42 frame at t with temperature1 set to v; nil leaves it out.
func frameAt(at time.Time, v *float64) keyvalue.Record {
	f := uvr42Frame(at, 0, false)
	if v == nil {
		delete(f, datalogger.KeyTemperature1)
	} else {
		f.Set(datalogger.KeyTemperature1, *v)
	}
	return f
}

func trend1(h *Handler) *float64 {
	return h.Status(views).Temperatures[0].Trend15m
}

func TestTrendAfter15Minutes(t *testing.T) {
	h := newHandler(0.5)
	for m := 0; m <= 15; m++ {
		v := 40 + float64(m)*0.2 // +3.0 K over 15 min
		h.checkAndUpdate(frameAt(t0.Add(time.Duration(m)*time.Minute), &v))
		if m < 15 {
			if tr := trend1(h); tr != nil {
				t.Fatalf("trend after %d min = %v, want none yet", m, *tr)
			}
		}
	}
	tr := trend1(h)
	if tr == nil || *tr < 2.99 || *tr > 3.01 {
		t.Fatalf("trend after 15 min = %v, want +3.0", tr)
	}

	// Falling: the sign follows.
	v := 30.0
	h.checkAndUpdate(frameAt(t0.Add(16*time.Minute), &v))
	if tr := trend1(h); tr == nil || *tr >= 0 {
		t.Errorf("falling trend = %v, want negative", tr)
	}
}

func TestNoTrendForSensorOutOfRange(t *testing.T) {
	h := newHandler(0.5)
	v := 40.0
	h.checkAndUpdate(frameAt(t0, nil)) // out of range 15 min ago
	h.checkAndUpdate(frameAt(t0.Add(15*time.Minute), &v))
	if tr := trend1(h); tr != nil {
		t.Errorf("trend = %v, want none: the value 15 min ago is missing", *tr)
	}
}

func TestNoTrendAfterGap(t *testing.T) {
	h := newHandler(0.5)
	v := 40.0
	h.checkAndUpdate(frameAt(t0, &v))
	h.checkAndUpdate(frameAt(t0.Add(time.Hour), &v)) // bus was down for an hour
	if tr := trend1(h); tr != nil {
		t.Errorf("trend = %v, want none after a gap", *tr)
	}
}
