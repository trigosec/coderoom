// Package history implements the conversation record list and its viewport.
package history

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/trigosec/coderoom/internal/ui/inlinefmt"
)

// RecordKind identifies the source and display style of a record.
type RecordKind int

// Record kind constants ordered from most to least common.
const (
	RecordKindUserInput   RecordKind = iota // text the user typed
	RecordKindAgentOutput                   // streaming response from an agent
	RecordKindSystem                        // lifecycle and routing notices
	RecordKindLog                           // agent diagnostic line (stderr)
	RecordKindReasoning                     // streaming internal reasoning trace from an agent
	RecordKindCommand                       // shell command execution item from an agent
)

// Record is a single displayable entry in the conversation history.
type Record struct {
	Kind     RecordKind
	Alias    string   // agent alias; empty for user input and system records
	Body     string   // accumulated content; grows during streaming
	Routing  []string // aliases shown in the footer (broadcast / direct send)
	Cmd      string   // shell command string; set on RecordKindCommand
	Cwd      string   // working directory for Cmd; set on RecordKindCommand
	ExitCode *int     // process exit code; nil until RecordKindCommand is sealed
}

var (
	systemStyle = lipgloss.NewStyle().Faint(true)
	promptStyle = lipgloss.NewStyle().Bold(true)
	logStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

const (
	promptPrefix    = "❯ "
	logPrefix       = "▸ "
	agentBullet     = "● "
	reasoningBullet = "◈ "
	commandBullet   = "$ "
	routingArrow    = "→ "
)

func renderRecord(r Record, width int, colors func(string) string) string {
	switch r.Kind {
	case RecordKindUserInput:
		return renderUserInput(r, width, colors)
	case RecordKindAgentOutput:
		return renderAgentOutput(r, width, colors)
	case RecordKindSystem:
		return systemStyle.Render(ansi.Wrap(r.Body, width, ""))
	case RecordKindLog:
		return logStyle.Render(renderLogBody(r.Body, width))
	case RecordKindReasoning:
		return renderReasoning(r, width, colors)
	case RecordKindCommand:
		return renderCommand(r, width, colors)
	}
	return r.Body
}

func renderLogBody(body string, width int) string {
	if body == "" {
		return ""
	}
	parts := strings.Split(body, "\n")
	out := make([]string, 0, len(parts))
	for i, line := range parts {
		// If body ends with a trailing newline, strings.Split includes a final
		// empty element; skip it to avoid rendering an orphaned "▸ " line.
		if i == len(parts)-1 && line == "" {
			continue
		}
		out = append(out, wrapLine(logPrefix+line, width, logPrefix))
	}
	return strings.Join(out, "\n")
}

func renderUserInput(r Record, width int, colors func(string) string) string {
	plain := promptPrefix + r.Body
	wrapped := wrapLine(plain, width, promptPrefix)
	// Style the prompt prefix on the first line.
	if strings.HasPrefix(wrapped, promptPrefix) {
		wrapped = promptStyle.Render(promptPrefix) + wrapped[len(promptPrefix):]
	}
	if len(r.Routing) > 0 {
		wrapped += "\n" + renderRoutingFooter(r.Routing, colors)
	}
	return wrapped
}

const agentBodyIndent = "  "

func renderAgentOutput(r Record, width int, colors func(string) string) string {
	color := colors(r.Alias)
	var header string
	var spanStyle lipgloss.Style
	if color != "" {
		spanStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(color))
		header = spanStyle.Render(agentBullet+r.Alias) + ":"
	} else {
		header = agentBullet + r.Alias + ":"
	}
	if r.Body == "" {
		return header
	}
	bodyText := r.Body
	if color != "" {
		bodyText = inlinefmt.Format(bodyText, spanStyle)
	}
	body := wrapLine(agentBodyIndent+bodyText, width, agentBodyIndent)
	return header + "\n\n" + body
}

func renderReasoning(r Record, width int, colors func(string) string) string {
	color := colors(r.Alias)
	var headerStyle lipgloss.Style
	if color != "" {
		headerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Faint(true)
	} else {
		headerStyle = lipgloss.NewStyle().Faint(true)
	}
	header := headerStyle.Render(reasoningBullet + r.Alias + " (thinking)")
	if r.Body == "" {
		return header
	}
	bodyText := r.Body
	if color != "" {
		// Keep the base text aligned with system messages, and use the
		// participant color only for inline emphasis spans (e.g. **bold**).
		bodyText = inlinefmt.FormatWithStyles(bodyText, systemStyle, lipgloss.NewStyle().Foreground(lipgloss.Color(color)))
	} else {
		bodyText = systemStyle.Render(bodyText)
	}
	body := wrapLine(agentBodyIndent+bodyText, width, agentBodyIndent)
	return header + "\n\n" + body
}

func renderCommand(r Record, width int, colors func(string) string) string {
	color := colors(r.Alias)
	cmd := r.Cmd
	if cmd == "" {
		cmd = "…"
	}
	var header string
	if color != "" {
		header = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(agentBullet+r.Alias) + ":"
	} else {
		header = agentBullet + r.Alias + ":"
	}

	commandPrompt := commandBullet
	if color != "" {
		commandPrompt = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(commandBullet)
	}
	commandLine := renderCommandLine(agentBodyIndent+commandPrompt, cmd, width)

	if r.Body == "" && r.ExitCode == nil {
		return header + "\n\n" + commandLine
	}

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n\n")
	sb.WriteString(commandLine)

	if r.Body != "" {
		sb.WriteString("\n\n")
		lines := strings.Split(strings.TrimRight(r.Body, "\n"), "\n")
		for i, line := range lines {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(wrapLine(agentBodyIndent+line, width, agentBodyIndent))
		}
	}

	if r.ExitCode != nil {
		sb.WriteString("\n")
		sb.WriteString(logStyle.Render(fmt.Sprintf("%sexit %d", agentBodyIndent, *r.ExitCode)))
	}

	return sb.String()
}

func renderRoutingFooter(aliases []string, colors func(string) string) string {
	parts := make([]string, len(aliases))
	indent := strings.Repeat(" ", ansi.StringWidth(promptPrefix))
	for i, alias := range aliases {
		color := colors(alias)
		if color != "" {
			parts[i] = indent + routingArrow + lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(alias)
		} else {
			parts[i] = indent + routingArrow + alias
		}
	}
	return strings.Join(parts, "    ")
}

func renderCommandLine(prefix string, cmd string, width int) string {
	if width <= 0 {
		return prefix + cmd
	}

	displayWidth := ansi.StringWidth(prefix)
	contentWidth := max(width-displayWidth, 1)
	wrapped := ansi.Wrap(cmd, contentWidth, "")
	parts := strings.Split(wrapped, "\n")

	indent := strings.Repeat(" ", displayWidth)
	for i := 1; i < len(parts); i++ {
		parts[i] = indent + parts[i]
	}
	return prefix + strings.Join(parts, "\n")
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
