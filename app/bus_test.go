package app

import (
	"testing"
	"time"

	"github.com/womat/tadl/app/service/health"
)

func TestParseDecoderInfo(t *testing.T) {
	hz := parseDecoderInfo("Decoder state: decoding data, Frequency: 50.00 Hz, Buffer overflow count: 0, Resync count: 3")
	if hz == nil || *hz != 50 {
		t.Errorf("decoding data: %v, want 50", hz)
	}
	if hz := parseDecoderInfo("Decoder state: discovering clock, Frequency: 0.00 Hz, Buffer overflow count: 0, Resync count: 0"); hz != nil {
		t.Errorf("discovering clock: %v, want nil", *hz)
	}
	if hz := parseDecoderInfo("something else"); hz != nil {
		t.Errorf("unknown text: %v, want nil", *hz)
	}
}

func TestSignalState(t *testing.T) {
	d := func(s int) *time.Duration { v := time.Duration(s) * time.Second; return &v }
	tests := []struct {
		record, frame, edge *time.Duration
		want                string
	}{
		{d(2), d(2), d(0), health.SignalReceiving},
		{d(60), d(2), d(0), health.SignalNoValidFrames},
		{nil, d(3), d(0), health.SignalNoValidFrames},
		// Edges arrive, but nothing comes out of them: the decoder, not the wiring.
		{d(60), d(60), d(0), health.SignalNotDecoded},
		{nil, nil, d(1), health.SignalNotDecoded},
		{d(60), d(60), d(60), health.SignalNone},
		{nil, nil, nil, health.SignalNone},
	}
	for _, tt := range tests {
		if got := signalState(tt.record, tt.frame, tt.edge); got != tt.want {
			t.Errorf("record=%v frame=%v edge=%v: %s, want %s", tt.record, tt.frame, tt.edge, got, tt.want)
		}
	}
}

func TestEdgeRate(t *testing.T) {
	if got := edgeRate(1000, 1820, 10*time.Second); got != 82 {
		t.Errorf("edgeRate = %v, want 82", got)
	}
	if got := edgeRate(1000, 1000, 0); got != 0 {
		t.Errorf("edgeRate over no time = %v, want 0", got)
	}
}

func TestDecoderStuck(t *testing.T) {
	now := time.Date(2026, 10, 21, 4, 50, 0, 0, time.UTC)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	never := time.Time{}

	tests := []struct {
		name                       string
		lastEdge, lastFrame, since time.Time
		want                       bool
	}{
		{"frames arrive", ago(0), ago(2 * time.Second), ago(time.Hour), false},
		{"edges, no frame for minutes", ago(0), ago(5 * time.Minute), ago(time.Hour), true},
		{"edges, never a frame", ago(0), never, ago(time.Hour), true},
		{"line silent: a reset would not help", ago(time.Minute), ago(5 * time.Minute), ago(time.Hour), false},
		{"no edge yet", never, never, ago(time.Hour), false},
		{"just started or reset", ago(0), ago(5 * time.Minute), ago(10 * time.Second), false},
	}
	for _, tt := range tests {
		if got := decoderStuck(now, tt.lastEdge, tt.lastFrame, tt.since); got != tt.want {
			t.Errorf("%s: decoderStuck = %v, want %v", tt.name, got, tt.want)
		}
	}
}
