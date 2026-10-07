package dlbus

import (
	"bytes"
	"slices"
	"testing"

	"github.com/womat/golib/manchester/decoder"
)

// bitsOf returns the bit stream a controller sends for the given frames: a
// SYNC of 16 high bits, then every byte with a low start bit, eight data bits
// LSB first and a high stop bit.
func bitsOf(frames ...[]byte) []decoder.Bit {
	var bits []decoder.Bit
	for _, f := range frames {
		for range 16 {
			bits = append(bits, decoder.High)
		}
		for _, b := range f {
			bits = append(bits, decoder.Low)
			for i := range 8 {
				bits = append(bits, decoder.Bit(b>>i&1))
			}
			bits = append(bits, decoder.High)
		}
	}
	return bits
}

// invert returns the bit stream as it arrives through an inverting line.
func invert(bits []decoder.Bit) []decoder.Bit {
	out := make([]decoder.Bit, len(bits))
	for i, b := range bits {
		if b == decoder.Invalid {
			out[i] = b
			continue
		}
		out[i] = decoder.High - b
	}
	return out
}

// run feeds bits through a Handler and returns the frames it delivered and
// its final stats.
func run(t *testing.T, bits []decoder.Bit) ([][]byte, Stats) {
	t.Helper()

	rx := make(chan decoder.Bit, len(bits))
	for _, b := range bits {
		rx <- b
	}
	close(rx)

	h := New()
	frames, err := h.Watch(rx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	var got [][]byte
	for f := range frames {
		got = append(got, f)
	}
	return got, h.Stats()
}

var (
	frameA = []byte{0x10, 0xd7, 0x00, 0xc2, 0x01, 0xff, 0xff, 0x00, 0x00, 0x20}
	frameB = []byte{0x10, 0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04, 0x00, 0x60}
	frameC = []byte{0x30, 0xaa, 0x55, 0x55, 0xaa, 0x00, 0xff, 0x00}
)

// TestFrames covers the plain case and the polarity detection: a frame is
// delivered when the next SYNC begins, whichever way round the line is.
func TestFrames(t *testing.T) {
	for _, inverted := range []bool{false, true} {
		bits := bitsOf(frameA, frameB, frameC)
		if inverted {
			bits = invert(bits)
		}

		got, stats := run(t, bits)

		// The last frame is still open: only a following SYNC would end it.
		if want := [][]byte{frameA, frameB}; !slices.EqualFunc(got, want, bytes.Equal) {
			t.Errorf("inverted=%v: frames % x, want % x", inverted, got, want)
		}
		if stats.InvertedLine != inverted {
			t.Errorf("inverted=%v: Stats.InvertedLine = %v", inverted, stats.InvertedLine)
		}
	}
}

// TestBitsBeforeTheFirstSyncAreDiscarded covers a receiver that starts in the
// middle of a frame.
func TestBitsBeforeTheFirstSyncAreDiscarded(t *testing.T) {
	tail := bitsOf(frameB)[40:] // the second half of a frame, without its SYNC
	got, _ := run(t, append(tail, bitsOf(frameA, frameC)...))

	if want := [][]byte{frameA}; !slices.EqualFunc(got, want, bytes.Equal) {
		t.Errorf("frames % x, want % x", got, want)
	}
}

// TestInvalidBitDropsTheFrame covers a timing error inside a frame: that frame
// is lost, the next one arrives.
func TestInvalidBitDropsTheFrame(t *testing.T) {
	bits := bitsOf(frameA, frameB, frameC, frameA)
	pos := len(bitsOf(frameA)) + 40 // inside frameB
	bits = slices.Insert(bits, pos, decoder.Invalid)

	got, _ := run(t, bits)

	if want := [][]byte{frameA, frameC}; !slices.EqualFunc(got, want, bytes.Equal) {
		t.Errorf("frames % x, want % x", got, want)
	}
}

// TestMissingStopBitIsAProtocolError covers a corrupted byte: the frame is
// dropped and counted.
func TestMissingStopBitIsAProtocolError(t *testing.T) {
	bits := bitsOf(frameA, frameB, frameC)
	bits[16+9] = decoder.Low // stop bit of the first byte of frameA

	got, stats := run(t, bits)

	if want := [][]byte{frameB}; !slices.EqualFunc(got, want, bytes.Equal) {
		t.Errorf("frames % x, want % x", got, want)
	}
	if stats.ProtocolErrors == 0 {
		t.Error("ProtocolErrors = 0, want the missing stop bit counted")
	}
}

// TestPolarityChange covers rewiring while running: the frame at the change
// may be lost, the frames after it are decoded in the new polarity.
func TestPolarityChange(t *testing.T) {
	bits := append(bitsOf(frameA, frameB), invert(bitsOf(frameC, frameA, frameB))...)

	got, stats := run(t, bits)

	if len(got) < 3 || !bytes.Equal(got[0], frameA) ||
		!bytes.Equal(got[len(got)-2], frameC) || !bytes.Equal(got[len(got)-1], frameA) {
		t.Errorf("frames % x, want frameA first and frameC, frameA last", got)
	}
	if !stats.InvertedLine {
		t.Error("Stats.InvertedLine = false after the change to an inverted line")
	}
}

// TestPolarity covers the SYNC detection itself: data bytes hold at most 9
// identical bits in a row and must never pass for a SYNC.
func TestPolarity(t *testing.T) {
	run := func(n int, b decoder.Bit) *polarity {
		var p polarity
		for range n {
			p.next(b)
		}
		p.next(decoder.High - b)
		return &p
	}

	if p := run(9, decoder.High); p.known {
		t.Error("9 high bits were taken for a SYNC")
	}
	if p := run(syncBits, decoder.High); !p.known || p.inverted {
		t.Errorf("%d high bits: known=%v inverted=%v, want a normal SYNC", syncBits, p.known, p.inverted)
	}
	if p := run(syncBits, decoder.Low); !p.known || !p.inverted {
		t.Errorf("%d low bits: known=%v inverted=%v, want an inverted SYNC", syncBits, p.known, p.inverted)
	}

	var p polarity
	for range syncBits - 1 {
		p.next(decoder.High)
	}
	p.next(decoder.Invalid)
	p.next(decoder.High)
	p.next(decoder.Low)
	if p.known {
		t.Error("a run broken by Invalid was taken for a SYNC")
	}
}
