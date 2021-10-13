// https://github.com/mrmorphic/hwio
// TODO: Use package gpiod https://github.com/warthog618/gpiod

// Package raspberry provides functionality for reading and writing to gpio pins
package raspberry

import "time"

// Edge represents the change in Pin level that triggers an interrupt.
type Edge string

const (
	// EdgeNone indicates no level transitions will trigger an interrupt
	EdgeNone Edge = "none"

	// EdgeRising indicates an interrupt is triggered when the Pin transitions from low to high.
	EdgeRising Edge = "rising"

	// EdgeFalling indicates an interrupt is triggered when the Pin transitions from high to low.
	EdgeFalling Edge = "falling"

	// EdgeBoth indicates an interrupt is triggered when the Pin changes level.
	EdgeBoth Edge = "both"
)

// GPIO is the interface implemented by a gpio memory
type GPIO interface {
	Close() error
	NewPin(int) (Pin, error)
}

// Pin is the interface implemented by a gpio pin
type Pin interface {
	SetBounceTime(time.Duration)
	Watch(Edge, func(Pin)) error
	Unwatch()
	Input()
	PullUp()
	PullDown()
	Pin() int
	Read() bool
	EmuEdge(Edge)
}
