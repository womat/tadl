// Package errwindow counts DL-Bus errors over the last 24 hours.
//
// The error counters of the bus are cumulative since start. A Window samples
// them, adds the increase to the current hour and sums the last 24 hours, so
// the web UI can show how many errors happened recently and when the last one was.
package errwindow

import (
	"sync"
	"time"
)

const hours = 24

// Counts holds one number per kind of error.
type Counts struct {
	RejectedFrames uint64 `json:"rejectedFrames"` // frames of the wrong size or device, or without a valid temperature
	DroppedFrames  uint64 `json:"droppedFrames"`  // frames discarded because the reader was busy
	ProtocolErrors uint64 `json:"protocolErrors"` // frames discarded for a missing stop bit
	DroppedEdges   uint64 `json:"droppedEdges"`   // GPIO edges lost before decoding
}

// Total returns the sum of all kinds.
func (c Counts) Total() uint64 {
	return c.RejectedFrames + c.DroppedFrames + c.ProtocolErrors + c.DroppedEdges
}

func (c Counts) add(o Counts) Counts {
	return Counts{
		RejectedFrames: c.RejectedFrames + o.RejectedFrames,
		DroppedFrames:  c.DroppedFrames + o.DroppedFrames,
		ProtocolErrors: c.ProtocolErrors + o.ProtocolErrors,
		DroppedEdges:   c.DroppedEdges + o.DroppedEdges,
	}
}

// since returns the increase from prev to c per kind. A counter that went down
// was reset, so its whole value counts as new.
func (c Counts) since(prev Counts) Counts {
	diff := func(now, before uint64) uint64 {
		if now < before {
			return now
		}
		return now - before
	}
	return Counts{
		RejectedFrames: diff(c.RejectedFrames, prev.RejectedFrames),
		DroppedFrames:  diff(c.DroppedFrames, prev.DroppedFrames),
		ProtocolErrors: diff(c.ProtocolErrors, prev.ProtocolErrors),
		DroppedEdges:   diff(c.DroppedEdges, prev.DroppedEdges),
	}
}

type bucket struct {
	hour   int64 // hours since the Unix epoch
	counts Counts
}

// Window sums errors over the last 24 hours. It is safe for concurrent use.
type Window struct {
	mu        sync.Mutex
	now       func() time.Time
	buckets   [hours]bucket
	last      Counts // cumulative counters at the previous Observe
	lastError time.Time
}

// New returns an empty Window.
func New() *Window {
	return &Window{now: time.Now}
}

// Observe takes the cumulative error counters and books their increase since
// the previous call in the current hour. The first call counts everything since
// start, which is what the counters hold then.
func (w *Window) Observe(cumulative Counts) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.now()
	added := cumulative.since(w.last)
	w.last = cumulative
	if added.Total() == 0 {
		return
	}

	hour := now.Unix() / 3600
	b := &w.buckets[hour%hours]
	if b.hour != hour {
		*b = bucket{hour: hour}
	}
	b.counts = b.counts.add(added)
	w.lastError = now
}

// Last24h returns the errors of the last 24 hours and when the last error was
// seen; the time is zero if there was none in that period.
func (w *Window) Last24h() (Counts, time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()

	hour := w.now().Unix() / 3600
	var sum Counts
	for _, b := range w.buckets {
		if b.hour > hour-hours && b.hour <= hour {
			sum = sum.add(b.counts)
		}
	}
	if sum.Total() == 0 {
		return sum, time.Time{}
	}
	return sum, w.lastError
}
