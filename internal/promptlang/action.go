// Package promptlang defines and parses the user-input language understood by
// Code Room. It is independent of the UI and session execution layers.
package promptlang

import "fmt"

// Statement is a sealed interface representing a parsed user input line.
// Only types defined in this package can implement it.
type Statement interface {
	isStatement()
}

// Invite invites a new agent into the session.
type Invite struct{ Alias string }

// Remove stops and removes an agent from the session.
type Remove struct{ Alias string }

// Cancel requests an agent to interrupt its current work.
type Cancel struct{ Alias string }

// Handoff transfers one agent's latest completed output to another.
type Handoff struct {
	FromAlias string
	ToAlias   string
}

// Send sends a message to one agent in the shared room (@alias text).
type Send struct {
	Alias string
	Text  string
}

// Broadcast sends a message to all agents.
type Broadcast struct{ Text string }

// Who displays the current agent roster.
type Who struct{}

// Help displays available commands.
type Help struct{}

// Quit exits the session.
type Quit struct{}

// DebugView prints a short viewport debug dump (development aid).
type DebugView struct{}

// DebugRows toggles viewport row-number overlay.
type DebugRows struct{}

// UnknownCommandError is returned for an unrecognized slash command.
type UnknownCommandError struct {
	Cmd string
}

func (e UnknownCommandError) Error() string {
	if e.Cmd == "" {
		return "unknown command"
	}
	return fmt.Sprintf("unknown command: %s", e.Cmd)
}

func (Invite) isStatement()    {}
func (Remove) isStatement()    {}
func (Cancel) isStatement()    {}
func (Handoff) isStatement()   {}
func (Send) isStatement()      {}
func (Broadcast) isStatement() {}
func (Who) isStatement()       {}
func (Help) isStatement()      {}
func (Quit) isStatement()      {}
func (DebugView) isStatement() {}
func (DebugRows) isStatement() {}
