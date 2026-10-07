package dlbus_test

import (
	"testing"
	"time"

	"github.com/womat/golib/keyvalue"
	"github.com/womat/golib/manchester/decoder"
	"github.com/womat/golib/manchester/encoder"
	"github.com/womat/tadl/pkg/datalogger"
	"github.com/womat/tadl/pkg/dlbus"
)

// line is a virtual DL-Bus between the golib encoder, acting as controller,
// and the receiving chain. Every level the encoder drives becomes an edge with
// a synthetic timestamp, so the bus runs at halfBit per half-bit whatever the
// encoder's own speed. invert models an optocoupler.
type line struct {
	halfBit time.Duration
	invert  bool
	now     time.Time
	level   encoder.Level
	started bool
	events  []decoder.Event
}

func (l *line) setValue(level encoder.Level) error {
	if l.invert {
		level = encoder.High - level
	}
	if l.started && level != l.level {
		edge := decoder.FallingEdge
		if level == encoder.High {
			edge = decoder.RisingEdge
		}
		l.events = append(l.events, decoder.Event{Time: l.now, Edge: edge})
	}
	l.level, l.started = level, true
	l.now = l.now.Add(l.halfBit)
	return nil
}

// transmit sends frames back to back, each with the 16-bit SYNC (two 0xff
// sync bytes), the way a controller does.
func transmit(t *testing.T, l *line, frames ...[]byte) {
	t.Helper()

	// The encoder's own clock only decides how fast the test runs; the bus
	// timing comes from the line.
	e, err := encoder.New(100_000, l.setValue, encoder.WithSyncBytes(2))
	if err != nil {
		t.Fatalf("encoder.New: %v", err)
	}
	for _, f := range frames {
		if _, err := e.Send(f); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}
	e.Wait()
	if err := e.Close(); err != nil {
		t.Fatalf("encoder.Close: %v", err)
	}
}

// receive runs the edges through the chain tadl uses: a decoder without a bit
// clock, the DL-Bus handler and the UVR42 data logger.
func receive(t *testing.T, events []decoder.Event) ([]keyvalue.Record, dlbus.Stats) {
	t.Helper()

	c := make(chan decoder.Event, len(events))
	for _, e := range events {
		c <- e
	}
	close(c)

	dec, err := decoder.New(c, 0, decoder.WithManchesterEncoding(decoder.IEEE), decoder.WithBufferSize(len(events)+16))
	if err != nil {
		t.Fatalf("decoder.New: %v", err)
	}
	defer dec.Close()

	bus := dlbus.New()
	frames, err := bus.Watch(dec.Bits())
	if err != nil {
		t.Fatalf("dlbus.Watch: %v", err)
	}

	dl := datalogger.NewUVR42()
	records, err := dl.Watch(frames)
	if err != nil {
		t.Fatalf("datalogger.Watch: %v", err)
	}

	var got []keyvalue.Record
	for r := range records {
		got = append(got, r)
	}
	return got, bus.Stats()
}

// TestUVR42ReachesTheDataLogger is the end-to-end test of the receiving chain:
// a UVR42 at 50 Hz, also with a clock off by 5 % either way, and also behind
// an inverting optocoupler, without telling the chain the bit clock or the
// polarity.
func TestUVR42ReachesTheDataLogger(t *testing.T) {
	temperatures := [4]float64{21.5, 45.3, -7.2, 0}
	frame := datalogger.FrameUVR42(temperatures, true, false)

	nominal := time.Second / 50 / 2
	for _, clock := range []struct {
		name    string
		halfBit time.Duration
	}{
		{"50 Hz", nominal},
		{"5% slow", nominal * 105 / 100},
		{"5% fast", nominal * 95 / 100},
		{"488 Hz", time.Second / 488 / 2},
	} {
		for _, invert := range []bool{false, true} {
			l := &line{halfBit: clock.halfBit, invert: invert, now: time.Now()}
			transmit(t, l, frame, frame, frame, frame)

			got, stats := receive(t, l.events)

			// The last frame stays open until a further SYNC would end it.
			if len(got) != 3 {
				t.Errorf("%s, inverted=%v: %d records, want 3", clock.name, invert, len(got))
				continue
			}
			for _, r := range got {
				for i, key := range []string{datalogger.KeyTemperature1, datalogger.KeyTemperature2,
					datalogger.KeyTemperature3, datalogger.KeyTemperature4} {
					if v := r.Float64(key); v != temperatures[i] {
						t.Errorf("%s, inverted=%v: %s = %v, want %v", clock.name, invert, key, v, temperatures[i])
					}
				}
				if !r.Bool(datalogger.KeyOut1) || r.Bool(datalogger.KeyOut2) {
					t.Errorf("%s, inverted=%v: outputs %v/%v, want true/false", clock.name, invert,
						r.Bool(datalogger.KeyOut1), r.Bool(datalogger.KeyOut2))
				}
			}
			if stats.InvertedLine != invert {
				t.Errorf("%s, inverted=%v: Stats.InvertedLine = %v", clock.name, invert, stats.InvertedLine)
			}
		}
	}
}
