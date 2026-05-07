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

func (Invite) isAction()    {}
func (Stop) isAction()      {}
func (Send) isAction()      {}
func (Broadcast) isAction() {}
func (Who) isAction()       {}
func (Help) isAction()      {}
func (Quit) isAction()      {}

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
	switch cmd {
	case "/invite":
		if rest == "" {
			return nil, fmt.Errorf("usage: /invite <alias>")
		}
		return Invite{Alias: rest}, nil
	case "/stop":
		if rest == "" {
			return nil, fmt.Errorf("usage: /stop <alias>")
		}
		return Stop{Alias: rest}, nil
	case "/who":
		return Who{}, nil
	case "/help":
		return Help{}, nil
	case "/quit":
		return Quit{}, nil
	default:
		return nil, fmt.Errorf("unknown command: %s", cmd)
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
