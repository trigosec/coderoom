// Package agent defines the interface for managing an AI agent process and
// provides implementations for supported CLI tools.
package agent

import (
	"errors"
	"fmt"
)

var (
	// ErrTurnInProgress is returned when a new turn is started while a prior turn
	// is still in flight.
	ErrTurnInProgress = errors.New("turn already in progress")
)

// StreamID identifies a logical message stream. Messages sharing an ID form one
// stream. The consumer uses it for grouping only — never parse or construct it
// outside the adapter.
//
// Consumer rule: compare StreamIDs for equality only. Never branch on prefixes,
// suffixes, or substrings. All semantic behavior is driven by content type and
// mode, not by the ID string.
type StreamID string

// Mode is the streaming lifecycle signal for a message.
type Mode int

// Streaming lifecycle signals for a Message.
const (
	ModeStream Mode = iota // fragment; a ModeFlush with the same content type follows
	ModeFlush              // stream is closed; carries the same content type as ModeStream
	ModeSingle             // standalone message; not part of a stream
)

// MessageContent is implemented only by types in this package.
type MessageContent interface {
	content()
}

// Message is a semantic unit of output from an agent turn.
type Message struct {
	StreamID StreamID
	Mode     Mode
	Content  MessageContent
}

// Accumulate merges next into m, returning the updated message.
// Used by consumers to build up stream state across ModeStream messages.
// Returns an error if the StreamIDs or content types are incompatible.
// When next is ModeFlush the returned message carries the same content as m
// (the flush payload is empty) with Mode set to ModeFlush.
func (m Message) Accumulate(next Message) (Message, error) {
	if m.StreamID != next.StreamID {
		return Message{}, fmt.Errorf("stream ID mismatch: %s vs %s", m.StreamID, next.StreamID)
	}
	switch c := m.Content.(type) {
	case Output:
		if nc, ok := next.Content.(Output); ok {
			return Message{StreamID: m.StreamID, Mode: next.Mode, Content: Output{Text: c.Text + nc.Text}}, nil
		}
	case Reasoning:
		if nc, ok := next.Content.(Reasoning); ok {
			return Message{StreamID: m.StreamID, Mode: next.Mode, Content: Reasoning{Text: c.Text + nc.Text}}, nil
		}
	case Command:
		if nc, ok := next.Content.(Command); ok {
			exitCode := nc.ExitCode
			if exitCode == nil {
				exitCode = c.ExitCode
			}
			output := c.Output
			if nc.overrideOutput != nil {
				output = *nc.overrideOutput
			} else {
				output += nc.Output
			}
			return Message{StreamID: m.StreamID, Mode: next.Mode, Content: Command{
				Command:  c.Command,
				Cwd:      c.Cwd,
				Output:   output,
				ExitCode: exitCode,
			}}, nil
		}
	}
	return Message{}, fmt.Errorf("incompatible content types: %T and %T", m.Content, next.Content)
}

// Content types. Value receivers are deliberate: all payload fields are
// reference types in Go, so copying a struct header is always cheap. Value
// receivers avoid nil pointer concerns and keep type switch cases free of *.

// Output carries agent-produced text during a turn.
type Output struct{ Text string }

// Reasoning carries an extended-thinking fragment from the agent.
type Reasoning struct{ Text string }

// Log carries a diagnostic or process-level log line.
type Log struct{ Text string }

// Command carries a shell command execution item from the agent.
// On ModeStream: Output holds a stdout+stderr delta fragment; Command and Cwd
// are populated only on the first fragment (item/started).
// ExitCode is populated on the last stream message.
type Command struct {
	Command  string
	Cwd      string
	Output   string
	ExitCode *int
	// overrideOutput, when non-nil, replaces any accumulated Output on the
	// next Accumulate call rather than being appended. It is never set on
	// accumulated results — only on messages produced by adapters that receive
	// a canonical complete output (e.g. aggregatedOutput from item/completed).
	overrideOutput *string
}

// CommandWithOverrideOutput returns a Command whose Output, when accumulated,
// replaces any previously accumulated delta output rather than appending to it.
// output == "" is treated as absent (returns a zero-value Command).
func CommandWithOverrideOutput(output string) Command {
	if output == "" {
		return Command{}
	}
	return Command{Output: output, overrideOutput: &output}
}

func (Output) content()    {}
func (Reasoning) content() {}
func (Log) content()       {}
func (Command) content()   {}

// Agent manages the lifecycle of and communication with an AI agent process.
type Agent interface {
	// Start launches the process and completes any required handshake.
	Start() error
	// Send writes a prompt to the agent and returns immediately.
	// Events arrive via Read().
	Send(prompt string) error
	// SendNotice delivers context to the agent without expecting a substantive
	// response. Implementations are responsible for instructing the model to
	// acknowledge with a minimal signal and suppressing the acknowledgment from
	// Read(). Non-compliant responses surface as reasoning rather than output.
	SendNotice(prompt string) error
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
// returning the accumulated output text.
//
// Turn completion is signalled by Output+ModeFlush. Turns are strictly
// sequential so there is at most one output stream in flight at a time.
func SendAndWait(a Agent, prompt string) (string, error) {
	if err := a.Send(prompt); err != nil {
		return "", fmt.Errorf("send: %w", err)
	}
	var sb []byte
	for {
		msg, err := a.Read()
		if err != nil {
			return "", fmt.Errorf("read: %w", err)
		}
		if c, ok := msg.Content.(Output); ok {
			switch msg.Mode {
			case ModeStream:
				sb = append(sb, c.Text...)
			case ModeFlush:
				return string(sb), nil
			default:
			}
		}
	}
}
