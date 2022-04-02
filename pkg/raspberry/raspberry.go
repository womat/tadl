// Package raspberry  is the watcher for gpio ports
package raspberry

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/warthog618/gpiod"
	"github.com/womat/debug"

	"tadl/pkg/port"
)

const (
	bounceTime = time.Microsecond * 100
)

// lines contains all open lines and must be global in package,
// the handler function handler(evt gpiod.LineEvent) needs the line handlers
var lines map[int]*Line
var ErrInvalidParam = fmt.Errorf("invalid parameters")

// Chip represents a single GPIO chip that controls a set of lines.
type Chip struct {
	gpiodChip *gpiod.Chip
}

// Line represents a single requested line.
type Line struct {
	gpiodLine *gpiod.Line
	gpiodChip *gpiod.Chip
	lastEvent time.Duration
	debounce  time.Duration
	// send edge changes to channel
	C chan port.Event
}

// Open opens a GPIO character device and initialize the global lines slice
func Open() (*Chip, error) {
	lines = map[int]*Line{}

	c, err := gpiod.NewChip("gpiochip0")
	chip := Chip{gpiodChip: c}
	return &chip, err
}

// Open requests control of a single line on a chip.
//   If granted, control is maintained until the Line is closed.
//   Watch the line for edge changes and send the changes after bounce timeout to chanel C.
//   There can only be one watcher on the pin at a time.
// config string: "gpio pullup|pulldown|none debounce(microseconds)"
//     e.g: "4 pullup 1000" or "4 none 0"

func (c *Chip) Open(param string) (*Line, error) {
	var err error
	var gpio, i int

	params := strings.Fields(param)
	if len(params) != 3 {
		return nil, ErrInvalidParam
	}

	gpio, err = strconv.Atoi(params[0])
	if err != nil {
		return nil, ErrInvalidParam
	}

	i, err = strconv.Atoi(params[2])
	if err != nil {
		return nil, ErrInvalidParam
	}

	if _, ok := lines[gpio]; ok {
		return nil, fmt.Errorf("line %v already used", gpio)
	}

	l := &Line{
		gpiodChip: c.gpiodChip,
		debounce:  time.Duration(i) * time.Microsecond,
		C:         make(chan port.Event)}

	switch params[1] {
	case "pullup":
		l.gpiodLine, err = c.gpiodChip.RequestLine(gpio, gpiod.WithEventHandler(handler),
			gpiod.WithBothEdges, gpiod.AsInput, gpiod.WithPullUp)
	case "pulldown":
		l.gpiodLine, err = c.gpiodChip.RequestLine(gpio, gpiod.WithEventHandler(handler),
			gpiod.WithBothEdges, gpiod.AsInput, gpiod.WithPullDown)
	case "none":
		l.gpiodLine, err = c.gpiodChip.RequestLine(gpio, gpiod.WithEventHandler(handler),
			gpiod.WithBothEdges, gpiod.AsInput)
	default:
		return nil, ErrInvalidParam
	}

	lines[gpio] = l
	return l, err
}

// Close releases the Chip.
//
// It does not release any lines which may be requested - they must be closed
// independently.
func (c *Chip) Close() (err error) {
	return c.gpiodChip.Close()
}

// Close releases all resources held by the requested line.
//
// Note that this includes waiting for any running event handler to return.
// As a consequence the Close must not be called from the context of the event
// handler - the Close should be called from a different goroutine.
func (l *Line) Close() (err error) {
	close(l.C)
	return l.gpiodLine.Close()
}

// handler check the bounce timeout and send the event to channel C
func handler(evt gpiod.LineEvent) {
	// check if map with pin struct exists
	line, ok := lines[evt.Offset]
	if !ok {
		return
	}

	var p time.Duration

	p, line.lastEvent = evt.Timestamp-line.lastEvent, evt.Timestamp

	if p < line.debounce {
		debug.ErrorLog.Printf("ignore bounce: %v:", p)
		return
	}

	event := port.Event{Timestamp: evt.Timestamp}

	switch evt.Type {
	case gpiod.LineEventFallingEdge:
		event.Type = port.FallingEdge
	case gpiod.LineEventRisingEdge:
		event.Type = port.RisingEdge
	}

	line.C <- event
}
