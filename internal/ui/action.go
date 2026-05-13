package ui

import (
	"fmt"
	"strings"
)

// Action is a sealed interface representing a parsed user input line.
// Only types defined in this package can implement it.
type Action interface {
	isAction()
}

// Invite invites a new agent into the session.
type Invite struct{ Alias string }

// Stop stops and removes an agent from the session.
type Stop struct{ Alias string }

// Cancel requests an agent to interrupt its current work.
type Cancel struct{ Alias string }

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

func (Invite) isAction()    {}
func (Stop) isAction()      {}
func (Cancel) isAction()    {}
func (Send) isAction()      {}
func (Broadcast) isAction() {}
func (Who) isAction()       {}
func (Help) isAction()      {}
func (Quit) isAction()      {}
func (DebugView) isAction() {}
func (DebugRows) isAction() {}

// Parse trims line and parses it into an Action.
// It returns an error for malformed input or unknown slash commands.
func Parse(line string) (Action, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, fmt.Errorf("input is empty")
	}
	if strings.HasPrefix(line, "/") {
		return parseSlash(line)
	}
	if strings.HasPrefix(line, "@") {
		return parseSend(line[1:])
	}
	return Broadcast{Text: line}, nil
}

func parseSlash(line string) (Action, error) {
	cmd, rest, _ := strings.Cut(line, " ")
	rest = strings.TrimSpace(rest)
	if cmd == "/invite" {
		if rest == "" {
			return nil, fmt.Errorf("usage: /invite <alias>")
		}
		return Invite{Alias: rest}, nil
	}
	if cmd == "/stop" {
		if rest == "" {
			return nil, fmt.Errorf("usage: /stop <alias>")
		}
		return Stop{Alias: rest}, nil
	}
	if cmd == "/cancel" {
		if rest == "" {
			return nil, fmt.Errorf("usage: /cancel <alias>")
		}
		return Cancel{Alias: rest}, nil
	}

	if a, ok := parseSlashNoArgs(cmd); ok {
		return a, nil
	}
	return nil, fmt.Errorf("unknown command: %s", cmd)
}

func parseSlashNoArgs(cmd string) (Action, bool) {
	switch cmd {
	case "/who":
		return Who{}, true
	case "/help":
		return Help{}, true
	case "/quit":
		return Quit{}, true
	case "/debugview":
		return DebugView{}, true
	case "/debugrows":
		return DebugRows{}, true
	default:
		return nil, false
	}
}

func parseSend(rest string) (Action, error) {
	alias, text, ok := strings.Cut(rest, " ")
	alias = strings.TrimSpace(alias)
	if !ok || alias == "" || strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("usage: @<alias> <text>")
	}
	return Send{Alias: alias, Text: strings.TrimSpace(text)}, nil
}
