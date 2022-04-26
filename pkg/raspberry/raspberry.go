// Package raspberry  is the watcher for gpio ports
package raspberry

import (
	"fmt"
	"github.com/womat/debug"
	"time"

	"github.com/warthog618/gpiod"
	"tadl/pkg/port"
)

var ErrInvalidParam = fmt.Errorf("invalid parameters")

// Chip represents a single GPIO chip that controls a set of lines.
type Chip struct {
	gpiodChip *gpiod.Chip
}

// Line represents a single requested line.
type Line struct {
	gpiodLine *gpiod.Line
	// send edge changes to channel
	C chan port.Event
}

// Open opens a GPIO character device and initialize the global lines slice
func Open() (*Chip, error) {
	c, err := gpiod.NewChip("gpiochip0")
	chip := Chip{gpiodChip: c}
	return &chip, err
}

// NewLine requests control of a single line on a chip.
//   If granted, control is maintained until the Line is closed.
//   Watch the line for edge changes and send the changes after bounce timeout to chanel C.
//   There can only be one watcher on the pin at a time.
func (c *Chip) NewLine(gpio int, terminator string, debounce time.Duration) (*Line, error) {
	var err error
	var collision bool
	var cnt int
	var lastEvent time.Duration

	line := &Line{
		C: make(chan port.Event, 100)}

	// handler check the bounce timeout and send the event to channel C
	handler := func(evt gpiod.LineEvent) {
		defer func() { collision = false }()

		if collision {
			debug.ErrorLog.Printf("handler collision detected")
		}

		collision = true

		if t := evt.Timestamp - lastEvent; t < debounce {
			cnt++
			debug.ErrorLog.Printf("time: %v bounce signal #%v (%v) detected (%v) - and ignored ;-)", evt.Timestamp, cnt, evt.Seqno, t)
			//	return
		} else {
			cnt = 0
		}

		lastEvent = evt.Timestamp

		switch evt.Type {
		case gpiod.LineEventFallingEdge:
			line.C <- port.Event{Type: port.FallingEdge, Timestamp: evt.Timestamp}
		case gpiod.LineEventRisingEdge:
			line.C <- port.Event{Type: port.RisingEdge, Timestamp: evt.Timestamp}
		}
	}

	switch terminator {
	case "pullup":
		line.gpiodLine, err = c.gpiodChip.RequestLine(gpio, gpiod.WithEventHandler(handler),
			gpiod.WithBothEdges, gpiod.AsInput, gpiod.WithPullUp)
	case "pulldown":
		line.gpiodLine, err = c.gpiodChip.RequestLine(gpio, gpiod.WithEventHandler(handler),
			gpiod.WithBothEdges, gpiod.AsInput, gpiod.WithPullDown)
	case "none":
		line.gpiodLine, err = c.gpiodChip.RequestLine(gpio, gpiod.WithEventHandler(handler),
			gpiod.WithBothEdges, gpiod.AsInput)
	default:
		return nil, ErrInvalidParam
	}

	return line, err
}

// Close releases the Chip.
//
// It does not release any lines which may be requested - they must be closed
// independently.
func (c *Chip) Close() error {
	return c.gpiodChip.Close()
}

// Close releases all resources held by the requested line.
//
// Note that this includes waiting for any running event handler to return.
// As a consequence the Close must not be called from the context of the event
// handler - the Close should be called from a different goroutine.
func (l *Line) Close() error {
	if err := l.gpiodLine.Close(); err != nil {
		return err
	}
	close(l.C)
	return nil
}
