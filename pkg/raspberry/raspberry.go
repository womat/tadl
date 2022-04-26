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
	gpiodLine  *gpiod.Line
	lastValue  int
	debouncing bool
	// send edge changes to channel
	C chan port.Event

	// channel to terminate debounce function
	quit chan struct{}
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
func (c *Chip) NewLine(gpio int, terminator string, debounceTime time.Duration) (*Line, error) {
	var err error
	eventChan := make(chan gpiod.LineEvent, 100)
	line := &Line{C: make(chan port.Event)}

	// handler check the bounce timeout and send the event to channel C
	handler := func(evt gpiod.LineEvent) {
		eventChan <- evt
	}

	// debouncing
	go func(interval time.Duration, input chan gpiod.LineEvent) {
		// If an event is received, wait the specified interval before calling the handler.
		// If another event is received before the interval has passed, store it and reset the timer.

		var item gpiod.LineEvent
		var lastEvent gpiod.LineEventType = -1

		timer := time.NewTimer(interval)

		for {
			select {
			case <-line.quit:
				return
			case item = <-input:
				timer.Reset(interval)
			case <-timer.C:
				// this function is only called once - with the last event
				if item.Type == lastEvent {
					break
				}

				lastEvent = item.Type
				switch item.Type {
				case gpiod.LineEventFallingEdge:
					line.C <- port.Event{Type: port.FallingEdge, Timestamp: item.Timestamp}
				case gpiod.LineEventRisingEdge:
					line.C <- port.Event{Type: port.RisingEdge, Timestamp: item.Timestamp}
				default:
					debug.ErrorLog.Printf("invalid pin value: %v", item.Type)
				}
			}
		}
	}(debounceTime, eventChan)

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
	l.quit <- struct{}{}
	close(l.C)
	return nil
}
