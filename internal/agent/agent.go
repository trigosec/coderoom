// Package agent defines the interface for managing an AI agent process and
// provides implementations for supported CLI tools.
package agent

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrTurnInProgress is returned when a new turn is started while a prior turn
	// is still in flight.
	ErrTurnInProgress = errors.New("turn already in progress")
)

// MessageKind identifies the type of an agent output message.
type MessageKind string

const (
	// MessageDelta is a streaming text fragment from the agent.
	MessageDelta MessageKind = "delta" // streaming text fragment
	// MessageDone marks completion of the current turn.
	MessageDone MessageKind = "done" // final message of a turn
	// MessageLog is a diagnostic line from the agent process (e.g. stderr).
	MessageLog MessageKind = "log" // diagnostic line from the agent process (e.g. stderr)
)

// Message is a semantic unit of output from an agent turn.
type Message struct {
	Kind MessageKind
	Text string
}

// Agent manages the lifecycle of and communication with an AI agent process.
type Agent interface {
	// Start launches the process and completes any required handshake.
	Start() error
	// Send writes a prompt to the agent and returns immediately.
	// Events arrive via Read().
	Send(prompt string) error
	// Read blocks until the next meaningful message arrives from the agent.
	// Returns an error if the process has exited or the turn has failed.
	Read() (Message, error)
	// Interrupt requests the agent to stop its current in-flight work.
	// Best-effort: implementations may use protocol-level cancellation or send
	// an OS interrupt signal (e.g. SIGINT) to the underlying process.
	// Interrupt must not terminate the agent process unless there is no safe
	// alternative.
	Interrupt() error
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
		msg, err := a.Read()
		if err != nil {
			return "", fmt.Errorf("read: %w", err)
		}
		switch msg.Kind {
		case MessageDelta:
			sb.WriteString(msg.Text)
		case MessageDone:
			return sb.String(), nil
		case MessageLog:
			// Intentionally ignored: SendAndWait returns only the agent's text output.
		default:
			// Unknown message kinds are ignored for forward compatibility.
		}
	}
}
