package collector

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/womat/golib/keyvalue"
	"github.com/womat/golib/mqtt"
	"github.com/womat/tadl/pkg/datalogger"
)

// fakePublisher records the published payloads and fails while err is set.
type fakePublisher struct {
	err      error
	payloads [][]byte
}

func (f *fakePublisher) Publish(msg mqtt.Message) error {
	if f.err != nil {
		return f.err
	}
	f.payloads = append(f.payloads, msg.Payload)
	return nil
}

var t0 = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func uvr42Frame(at time.Time, t1 float64, out1 bool) keyvalue.Record {
	r := keyvalue.NewRecord()
	r.Set(datalogger.KeyTimestamp, at)
	r.Set(datalogger.KeyTemperature1, t1)
	r.Set(datalogger.KeyTemperature2, 45.3)
	r.Set(datalogger.KeyTemperature3, -7.2)
	r.Set(datalogger.KeyTemperature4, 0.0)
	r.Set(datalogger.KeyOut1, out1)
	r.Set(datalogger.KeyOut2, false)
	return r
}

func newHandler(minDelta float64) *Handler {
	h := New(Config{
		PublishInterval: time.Minute,
		MinDeltaTemp:    minDelta,
		Topic:           "test/uvr42",
		StaleAfter:      3 * time.Minute,
	}, datalogger.UVR42)
	h.now = func() time.Time { return t0 }
	return h
}

// feed runs one frame through checkAndUpdate and publishes it when due, like Run.
func feed(t *testing.T, h *Handler, pub *fakePublisher, f keyvalue.Record) bool {
	t.Helper()
	changed, err := h.checkAndUpdate(f)
	if err != nil {
		t.Fatalf("checkAndUpdate: %v", err)
	}
	if changed {
		_ = h.PublishFrame(pub)
	}
	return changed
}

func TestFirstFramePublished(t *testing.T) {
	h, pub := newHandler(0.5), &fakePublisher{}
	if !feed(t, h, pub, uvr42Frame(t0, 21.5, false)) || len(pub.payloads) != 1 {
		t.Fatalf("first frame not published, payloads=%d", len(pub.payloads))
	}
}

// A slow drift must add up against the last published frame instead of being
// compared with its predecessor only.
func TestDriftAddsUpToMinDelta(t *testing.T) {
	h, pub := newHandler(0.5), &fakePublisher{}
	for i := range 6 {
		temp := float64(215+i) / 10 // 21.5, 21.6, … 22.0 as decoded from the bus
		changed := feed(t, h, pub, uvr42Frame(t0, temp, false))
		want := i == 0 || i == 5
		if changed != want {
			t.Errorf("frame %d (%.1f °C): changed=%v, want %v", i+1, temp, changed, want)
		}
	}
	if len(pub.payloads) != 2 {
		t.Errorf("published %d frames, want 2", len(pub.payloads))
	}
}

func TestMinDeltaZeroOnlyOutputsTrigger(t *testing.T) {
	h, pub := newHandler(0), &fakePublisher{}
	feed(t, h, pub, uvr42Frame(t0, 21.5, false))
	if feed(t, h, pub, uvr42Frame(t0, 30, false)) {
		t.Error("temperature change triggered a publish with minDeltaTemp 0")
	}
	if !feed(t, h, pub, uvr42Frame(t0, 30, true)) {
		t.Error("output change did not trigger a publish")
	}
}

func TestFailedPublishIsRetried(t *testing.T) {
	h, pub := newHandler(0.5), &fakePublisher{err: mqtt.ErrNotConnected}
	feed(t, h, pub, uvr42Frame(t0, 21.5, false))
	pub.err = nil
	if !feed(t, h, pub, uvr42Frame(t0, 21.5, false)) {
		t.Error("frame after a failed publish was not published")
	}
}

func TestSensorAppearingOrDisappearingTriggers(t *testing.T) {
	h, pub := newHandler(0.5), &fakePublisher{}
	feed(t, h, pub, uvr42Frame(t0, 21.5, false))

	f := uvr42Frame(t0, 21.5, false)
	delete(f, datalogger.KeyTemperature2)
	if !feed(t, h, pub, f) {
		t.Error("sensor dropping out did not trigger a publish")
	}
}

func TestNoDataBeforeFirstFrame(t *testing.T) {
	h, pub := newHandler(0.5), &fakePublisher{}
	if err := h.PublishFrame(pub); !errors.Is(err, ErrNoData) {
		t.Fatalf("PublishFrame without data: err=%v, want ErrNoData", err)
	}
	if len(pub.payloads) != 0 {
		t.Fatalf("published %q without data", pub.payloads[0])
	}
	if _, ok := h.Current(); ok {
		t.Error("Current reports data before the first frame")
	}
}

func TestStaleFrameNotPublished(t *testing.T) {
	h, pub := newHandler(0.5), &fakePublisher{}
	feed(t, h, pub, uvr42Frame(t0, 21.5, false))

	h.now = func() time.Time { return t0.Add(3*time.Minute + time.Second) }
	if err := h.PublishFrame(pub); !errors.Is(err, ErrNoData) {
		t.Errorf("PublishFrame with stale data: err=%v, want ErrNoData", err)
	}
	if _, ok := h.Current(); ok {
		t.Error("Current reports stale data as current")
	}
}

func TestPublishedPayload(t *testing.T) {
	h, pub := newHandler(0.5), &fakePublisher{}
	feed(t, h, pub, uvr42Frame(t0, -7.2, true))

	var got map[string]any
	if err := json.Unmarshal(pub.payloads[0], &got); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if got[datalogger.KeyTemperature1] != -7.2 || got[datalogger.KeyOut1] != true {
		t.Errorf("payload = %v", got)
	}
}

func TestFrameWithoutTimestampRejected(t *testing.T) {
	h := newHandler(0.5)
	if _, err := h.checkAndUpdate(keyvalue.NewRecord()); err == nil {
		t.Error("frame without timestamp accepted")
	}
}
