package promptlang

import (
	"fmt"
	"strings"
)

// Parse trims line and parses it into a Statement.
// It returns an error for malformed input or unknown slash commands.
func Parse(line string) (Statement, error) {
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

func parseSlash(line string) (Statement, error) {
	cmd, rest, _ := strings.Cut(line, " ")
	rest = strings.TrimSpace(rest)
	if statement, matched, err := parseSlashWithArgs(cmd, rest); matched {
		return statement, err
	}
	if statement, ok := parseSlashNoArgs(cmd); ok {
		return statement, nil
	}
	return nil, UnknownCommandError{Cmd: cmd}
}

func parseSlashWithArgs(cmd, rest string) (Statement, bool, error) {
	switch cmd {
	case "/invite", "/remove", "/cancel":
		statement, err := parseAliasStatement(cmd, rest)
		return statement, true, err
	case "/handoff":
		fromAlias, toAlias, err := parseHandoffArgs(rest)
		if err != nil {
			return nil, true, err
		}
		return Handoff{FromAlias: fromAlias, ToAlias: toAlias}, true, nil
	case "/shell":
		if rest == "" {
			return nil, true, fmt.Errorf("usage: /shell <program>")
		}
		return Shell{Program: rest}, true, nil
	default:
		return nil, false, nil
	}
}

func parseAliasStatement(cmd, alias string) (Statement, error) {
	if alias == "" {
		return nil, fmt.Errorf("usage: %s <alias>", cmd)
	}
	switch cmd {
	case "/invite":
		return Invite{Alias: alias}, nil
	case "/remove":
		return Remove{Alias: alias}, nil
	default:
		return Cancel{Alias: alias}, nil
	}
}

func parseHandoffArgs(rest string) (string, string, error) {
	parts := strings.Fields(rest)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("usage: /handoff <from> <to>")
	}
	return parts[0], parts[1], nil
}

func parseSlashNoArgs(cmd string) (Statement, bool) {
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

func parseSend(rest string) (Statement, error) {
	alias, text, ok := strings.Cut(rest, " ")
	alias = strings.TrimSpace(alias)
	if !ok || alias == "" || strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("usage: @<alias> <text>")
	}
	return Send{Alias: alias, Text: strings.TrimSpace(text)}, nil
}
