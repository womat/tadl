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
		record, frame *time.Duration
		want          string
	}{
		{d(2), d(2), health.SignalReceiving},
		{d(60), d(2), health.SignalNoValidFrames},
		{nil, d(3), health.SignalNoValidFrames},
		{d(60), d(60), health.SignalNone},
		{nil, nil, health.SignalNone},
	}
	for _, tt := range tests {
		if got := signalState(tt.record, tt.frame); got != tt.want {
			t.Errorf("record=%v frame=%v: %s, want %s", tt.record, tt.frame, got, tt.want)
		}
	}
}
