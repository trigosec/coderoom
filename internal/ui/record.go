package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/trigosec/coderoom/internal/ui/inlinefmt"
)

type recordKind int

const (
	recordKindUserInput   recordKind = iota // text the user typed
	recordKindAgentOutput                   // streaming response from an agent
	recordKindSystem                        // lifecycle and routing notices
	recordKindLog                           // agent diagnostic line (stderr)
)

type record struct {
	kind    recordKind
	alias   string   // agent alias; empty for user input and system records
	body    string   // accumulated content; grows during streaming
	routing []string // aliases shown in the footer (broadcast / direct send)
}

var (
	systemStyle = lipgloss.NewStyle().Faint(true)
	promptStyle = lipgloss.NewStyle().Bold(true)
	logStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

const (
	promptPrefix = "❯ "
	logPrefix    = "▸ "
	agentBullet  = "● "
	routingArrow = "→ "
)

func renderRecord(r record, width int, colors func(string) string) string {
	switch r.kind {
	case recordKindUserInput:
		return renderUserInput(r, width, colors)
	case recordKindAgentOutput:
		return renderAgentOutput(r, width, colors)
	case recordKindSystem:
		return systemStyle.Render(ansi.Wrap(r.body, width, ""))
	case recordKindLog:
		return logStyle.Render(wrapLine(logPrefix+r.body, width, logPrefix))
	}
	return r.body
}

func renderUserInput(r record, width int, colors func(string) string) string {
	plain := promptPrefix + r.body
	wrapped := wrapLine(plain, width, promptPrefix)
	// Style the prompt prefix on the first line.
	if strings.HasPrefix(wrapped, promptPrefix) {
		wrapped = promptStyle.Render(promptPrefix) + wrapped[len(promptPrefix):]
	}
	if len(r.routing) > 0 {
		wrapped += "\n" + renderRoutingFooter(r.routing, colors)
	}
	return wrapped
}

const agentBodyIndent = "  "

func renderAgentOutput(r record, width int, colors func(string) string) string {
	color := colors(r.alias)
	var header string
	var spanStyle lipgloss.Style
	if color != "" {
		spanStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(color))
		header = spanStyle.Render(agentBullet+r.alias) + ":"
	} else {
		header = agentBullet + r.alias + ":"
	}
	if r.body == "" {
		return header
	}
	bodyText := r.body
	if color != "" {
		bodyText = inlinefmt.Format(bodyText, spanStyle)
	}
	body := wrapLine(agentBodyIndent+bodyText, width, agentBodyIndent)
	return header + "\n\n" + body
}

func renderRoutingFooter(aliases []string, colors func(string) string) string {
	parts := make([]string, len(aliases))
	for i, alias := range aliases {
		color := colors(alias)
		if color != "" {
			parts[i] = routingArrow + lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(alias)
		} else {
			parts[i] = routingArrow + alias
		}
	}
	return strings.Join(parts, "    ")
}

// wrapLine wraps line to width. If prefix is non-empty, continuation lines
// are indented to align with the first content column after the prefix.
// Requires that line starts with prefix when prefix is non-empty.
func wrapLine(line string, width int, prefix string) string {
	if width <= 0 {
		return line
	}
	if prefix == "" {
		return ansi.Wrap(line, width, "")
	}
	if !strings.HasPrefix(line, prefix) {
		return ansi.Wrap(line, width, "")
	}
	displayWidth := ansi.StringWidth(prefix)
	indent := strings.Repeat(" ", displayWidth)
	contentWidth := max(width-displayWidth, 1)
	wrapped := ansi.Wrap(line[len(prefix):], contentWidth, "")
	parts := strings.Split(wrapped, "\n")
	for i := 1; i < len(parts); i++ {
		parts[i] = indent + parts[i]
	}
	return prefix + strings.Join(parts, "\n")
}
