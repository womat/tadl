// Package port holds the definition of a physical port
package port

import "time"

// EventType indicates the type of change to the line active state.
//
// Note that for active low lines a low line level results in a high active
// state.
type EventType int

const (
	_ EventType = iota
	// RisingEdge indicates an inactive to active event (low to high).
	RisingEdge
	// FallingEdge indicates an active to inactive event (high to low).
	FallingEdge
)

type Event struct {
	// Timestamp indicates the time the event was detected.
	Timestamp time.Duration
	// The type of state change event this structure represents.
	Type EventType
}

type StateType int

const (
	// High indicates a logical 1.
	High StateType = 1
	// Low indicates a logical 0.
	Low StateType = 0
	// Invalid indicates an unknown or invalid state.
	Invalid StateType = -1
)
