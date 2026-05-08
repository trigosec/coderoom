// Package agent defines the interface for managing an AI agent process and
// provides implementations for supported CLI tools.
package agent

import (
	"fmt"
	"strings"
)

// Event is a semantic unit of output from an agent turn.
// At most one field is non-zero per returned event.
type Event struct {
	Delta string // text fragment; empty on non-delta events
	Done  bool   // true on the final event of a turn
	Log   string // diagnostic line from the agent process (e.g. stderr)
}

// Agent manages the lifecycle of and communication with an AI agent process.
type Agent interface {
	// Start launches the process and completes any required handshake.
	Start() error
	// Send writes a prompt to the agent and returns immediately.
	// Events arrive via Read().
	Send(prompt string) error
	// Read blocks until the next meaningful event arrives from the agent.
	// Returns an error if the process has exited or the turn has failed.
	Read() (Event, error)
	// Stop kills the process and blocks until it is fully reaped.
	// May be called from a different goroutine to interrupt a blocked Read.
	Stop() error
}

// SendAndWait sends a prompt and blocks until the turn is complete,
// returning the accumulated response text.
func SendAndWait(a Agent, prompt string) (string, error) {
	if err := a.Send(prompt); err != nil {
		return "", fmt.Errorf("send: %w", err)
	}
	var sb strings.Builder
	for {
		ev, err := a.Read()
		if err != nil {
			return "", fmt.Errorf("read: %w", err)
		}
		sb.WriteString(ev.Delta)
		if ev.Done {
			return sb.String(), nil
		}
	}
}
