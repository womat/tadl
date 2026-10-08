package errwindow

import (
	"testing"
	"time"
)

func newAt(t *time.Time) *Window {
	w := New()
	w.now = func() time.Time { return *t }
	return w
}

func TestNoErrors(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	w := newAt(&now)
	w.Observe(Counts{})
	w.Observe(Counts{})
	if sum, last := w.Last24h(); sum.Total() != 0 || !last.IsZero() {
		t.Errorf("sum=%+v last=%v, want nothing", sum, last)
	}
}

func TestIncreaseIsBookedAndLastErrorSet(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	w := newAt(&now)
	w.Observe(Counts{ProtocolErrors: 2})

	now = now.Add(time.Minute)
	w.Observe(Counts{ProtocolErrors: 2}) // no increase
	now = now.Add(time.Minute)
	w.Observe(Counts{ProtocolErrors: 3, DroppedEdges: 4})
	errAt := now

	now = now.Add(10 * time.Minute)
	w.Observe(Counts{ProtocolErrors: 3, DroppedEdges: 4})

	sum, last := w.Last24h()
	if sum.ProtocolErrors != 3 || sum.DroppedEdges != 4 || sum.Total() != 7 {
		t.Errorf("sum = %+v, want 3 protocol errors and 4 dropped edges", sum)
	}
	if !last.Equal(errAt) {
		t.Errorf("last error = %v, want %v", last, errAt)
	}
}

func TestErrorsLeaveAfter24h(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 30, 0, 0, time.UTC)
	w := newAt(&now)
	w.Observe(Counts{RejectedFrames: 5})

	now = now.Add(23 * time.Hour)
	w.Observe(Counts{RejectedFrames: 6})
	if sum, _ := w.Last24h(); sum.RejectedFrames != 6 {
		t.Errorf("after 23 h: %d rejected, want 6", sum.RejectedFrames)
	}

	now = now.Add(time.Hour)
	if sum, _ := w.Last24h(); sum.RejectedFrames != 1 {
		t.Errorf("after 24 h: %d rejected, want 1", sum.RejectedFrames)
	}

	now = now.Add(23 * time.Hour)
	if sum, last := w.Last24h(); sum.Total() != 0 || !last.IsZero() {
		t.Errorf("after 47 h: sum=%+v last=%v, want nothing", sum, last)
	}
}

func TestCounterReset(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	w := newAt(&now)
	w.Observe(Counts{DroppedFrames: 10})
	w.Observe(Counts{DroppedFrames: 2}) // counters restarted
	if sum, _ := w.Last24h(); sum.DroppedFrames != 12 {
		t.Errorf("dropped = %d, want 12", sum.DroppedFrames)
	}
}
