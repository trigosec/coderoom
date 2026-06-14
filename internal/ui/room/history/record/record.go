package record

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/ui/inlinefmt"
)

// Kind identifies the source and display style of a record.
type Kind int

// Record kind constants ordered from most to least common.
const (
	KindUserInput   Kind = iota // text the user typed
	KindAgentOutput             // streaming response from an agent
	KindSystem                  // lifecycle and routing notices
	KindLog                     // agent diagnostic line (stderr)
	KindReasoning               // streaming internal reasoning trace from an agent
	KindCommand                 // shell command execution item from an agent
	KindFileChange              // file patch/diff item from an agent
)

// Record is a single displayable entry in the conversation history.
//
// Text is used both for non-agent records (user/system/log) and as a cached body
// for agent-backed records (Msg != nil).
type Record struct {
	Kind    Kind
	Alias   string   // agent alias; empty for user input and system records
	Routing []string // aliases shown in the footer (broadcast / direct send)
	Text    string   // body text or cached body derived from Msg
	Msg     *agent.Message

	renderCache struct {
		key      RenderKey
		rendered string
		valid    bool
	}
}

// RenderMode controls how Record.Render formats output.
type RenderMode int

const (
	// RenderViewport wraps and truncates based on viewport width.
	RenderViewport RenderMode = iota
	// RenderTranscript disables wrapping and emits full content.
	RenderTranscript
)

// RenderContext carries caller-provided rendering policy and dependencies.
type RenderContext struct {
	Key RenderKey
	// ColorForAlias returns a lipgloss color string for an active alias, or "".
	ColorForAlias func(alias string) string
}

// RenderKey is the comparable subset of RenderContext that affects output.
// Callers should bump ColorVersion whenever color resolution may change (theme
// changes, agent departed) to invalidate per-record cached rendering.
type RenderKey struct {
	Mode         RenderMode
	Width        int
	ColorVersion uint64
}

var (
	systemStyle = lipgloss.NewStyle().Faint(true)
	promptStyle = lipgloss.NewStyle().Bold(true)
	logStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

const (
	promptPrefix     = "❯ "
	logPrefix        = "▸ "
	agentBullet      = "● "
	reasoningBullet  = "◈ "
	commandBullet    = "$ "
	fileChangeBullet = "✎ "
	routingArrow     = "→ "
)

// NewAgent constructs a record backed by an agent.Message. It caches the record
// body in Text for efficient re-rendering.
func NewAgent(alias string, msg agent.Message) Record {
	msgCopy := msg
	return Record{
		Kind:  kindFor(msg),
		Alias: alias,
		Msg:   &msgCopy,
		Text:  bodyFrom(msg),
	}
}

// Accumulate merges next into r.Msg and returns the updated record.
func (r Record) Accumulate(next agent.Message) (Record, error) {
	if r.Msg == nil {
		return Record{}, fmt.Errorf("record has no message")
	}
	accumulated, err := r.Msg.Accumulate(next)
	if err != nil {
		return Record{}, fmt.Errorf("accumulate message: %w", err)
	}
	accumulatedCopy := accumulated
	r.Msg = &accumulatedCopy
	r.Text = bodyFrom(accumulated)
	r.renderCache.valid = false
	return r, nil
}

// Render returns the record rendered for the given context.
func (r Record) Render(ctx RenderContext) string {
	width := ctx.Key.Width
	if ctx.Key.Mode == RenderTranscript {
		width = 0
	}
	colors := ctx.ColorForAlias
	if colors == nil {
		colors = func(string) string { return "" }
	}

	switch ctx.Key.Mode {
	case RenderTranscript:
		return renderTranscript(r, colors)
	default:
		return renderViewport(r, width, colors)
	}
}

// RenderCached returns the rendered string and an updated Record containing a
// cached render result. Callers should store the returned Record if they want
// caching to persist across renders.
func (r Record) RenderCached(ctx RenderContext) (string, Record) {
	key := ctx.Key
	if key.Mode == RenderTranscript {
		key.Width = 0
	}
	if r.renderCache.valid && r.renderCache.key == key {
		return r.renderCache.rendered, r
	}
	rendered := r.Render(ctx)
	r.renderCache.key = key
	r.renderCache.rendered = rendered
	r.renderCache.valid = true
	return rendered, r
}

func renderViewport(r Record, width int, colors func(string) string) string {
	switch r.Kind {
	case KindUserInput:
		return renderUserInput(r, width, colors)
	case KindAgentOutput:
		return renderAgentOutput(r, width, colors)
	case KindSystem:
		return systemStyle.Render(ansi.Wrap(r.Text, width, ""))
	case KindLog:
		return logStyle.Render(renderLogBody(r.Text, width))
	case KindReasoning:
		return renderReasoning(r, width, colors)
	case KindCommand:
		return renderCommand(r, width, colors)
	case KindFileChange:
		return renderFileChange(r, width, colors)
	}
	return r.Text
}

func renderTranscript(r Record, colors func(string) string) string {
	if r.Kind == KindCommand {
		return renderCommandTranscript(r, colors)
	}
	if r.Kind == KindFileChange {
		return renderFileChangeTranscript(r, colors)
	}
	return renderViewport(r, 0, colors)
}

func renderLogBody(body string, width int) string {
	if body == "" {
		return ""
	}
	parts := strings.Split(body, "\n")
	out := make([]string, 0, len(parts))
	for i, line := range parts {
		// strings.Split("x\n", "\n") includes a final empty element; skip it so we
		// don't render a dangling prefixed line for trailing newlines.
		if i == len(parts)-1 && line == "" {
			continue
		}
		out = append(out, wrapLine(logPrefix+line, width, logPrefix))
	}
	return strings.Join(out, "\n")
}

func renderUserInput(r Record, width int, colors func(string) string) string {
	plain := promptPrefix + r.Text
	wrapped := wrapLine(plain, width, promptPrefix)
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
	body := bodyFromRecord(r)
	if body == "" {
		return header
	}
	bodyText := body
	if color != "" {
		bodyText = inlinefmt.Format(bodyText, spanStyle)
	}
	wrapped := wrapLine(agentBodyIndent+bodyText, width, agentBodyIndent)
	return header + "\n\n" + wrapped
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
	body := bodyFromRecord(r)
	if body == "" {
		return header
	}
	bodyText := body
	if color != "" {
		// Keep base text aligned with system messages; use participant color only
		// for inline emphasis.
		bodyText = inlinefmt.FormatWithStyles(bodyText, systemStyle, lipgloss.NewStyle().Foreground(lipgloss.Color(color)))
	} else {
		bodyText = systemStyle.Render(bodyText)
	}
	wrapped := wrapLine(agentBodyIndent+bodyText, width, agentBodyIndent)
	return header + "\n\n" + wrapped
}

func bodyFrom(msg agent.Message) string {
	switch c := msg.Content.(type) {
	case agent.Output:
		return c.Text
	case agent.Reasoning:
		return c.Text
	case agent.Command:
		return c.Output
	case agent.FileChangeSet:
		return FormatFileChangeBody(c.Changes)
	}
	return ""
}

func kindFor(msg agent.Message) Kind {
	switch msg.Content.(type) {
	case agent.Reasoning:
		return KindReasoning
	case agent.Command:
		return KindCommand
	case agent.FileChangeSet:
		return KindFileChange
	}
	return KindAgentOutput
}

func bodyFromRecord(r Record) string {
	if r.Msg == nil {
		return ""
	}
	if r.Text != "" {
		return r.Text
	}
	return bodyFrom(*r.Msg)
}

func commandFieldsFromRecord(r Record) (cmd string, output string, exitCode *int) {
	if r.Msg == nil {
		return "", "", nil
	}
	c, ok := r.Msg.Content.(agent.Command)
	if !ok {
		return "", "", nil
	}
	return c.Command, c.Output, c.ExitCode
}

func renderCommand(r Record, width int, colors func(string) string) string {
	color := colors(r.Alias)
	cmd, output, exitCode := commandFieldsFromRecord(r)
	if cmd == "" {
		cmd = "…"
	}
	header := renderParticipantHeader(r.Alias, color)

	commandPrompt := renderCommandPrompt(color)
	commandLine, cmdTruncated := renderCommandSummaryLine(agentBodyIndent+commandPrompt, cmd, width)
	cmdHasNewline := strings.Contains(cmd, "\n")

	if output == "" && exitCode == nil {
		return renderPendingCommand(header, commandLine, cmdHasNewline || cmdTruncated)
	}

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n\n")
	sb.WriteString(commandLine)

	preview, remaining := renderCommandOutputPreview(output, width)
	if preview != "" {
		sb.WriteString("\n\n")
		sb.WriteString(preview)
	}

	if shouldRenderCommandHint(remaining, cmdHasNewline, cmdTruncated) {
		sb.WriteString("\n")
		sb.WriteString(logStyle.Render(commandDetailsHint(commandDetailsHintParams{
			cmdTruncated: cmdTruncated || cmdHasNewline,
			moreLines:    remaining,
		})))
	}

	if exitCode != nil {
		sb.WriteString("\n")
		sb.WriteString(logStyle.Render(fmt.Sprintf("%sexit %d", agentBodyIndent, *exitCode)))
	}

	return sb.String()
}

func renderParticipantHeader(alias string, color string) string {
	if color == "" {
		return agentBullet + alias + ":"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(agentBullet+alias) + ":"
}

func renderCommandPrompt(color string) string {
	if color == "" {
		return commandBullet
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(commandBullet)
}

func renderFileChangePrompt(color string) string {
	if color == "" {
		return fileChangeBullet
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(fileChangeBullet)
}

func renderPendingCommand(header string, commandLine string, showHint bool) string {
	if !showHint {
		return header + "\n\n" + commandLine
	}
	return header + "\n\n" + commandLine + "\n" + logStyle.Render(commandDetailsHint(commandDetailsHintParams{
		cmdTruncated: true,
	}))
}

func shouldRenderCommandHint(moreLines int, cmdHasNewline bool, cmdTruncated bool) bool {
	return moreLines > 0 || cmdHasNewline || cmdTruncated
}

func renderCommandTranscript(r Record, colors func(string) string) string {
	color := colors(r.Alias)
	cmd, output, exitCode := commandFieldsFromRecord(r)
	if cmd == "" {
		cmd = "…"
	}
	header := renderParticipantHeader(r.Alias, color)
	commandPrompt := renderCommandPrompt(color)
	commandLine := renderCommandLine(agentBodyIndent+commandPrompt, cmd, 0)

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n\n")
	sb.WriteString(commandLine)

	if output != "" {
		sb.WriteString("\n\n")
		lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
		for i, line := range lines {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(agentBodyIndent + line)
		}
	}

	if exitCode != nil {
		sb.WriteString("\n")
		sb.WriteString(logStyle.Render(fmt.Sprintf("%sexit %d", agentBodyIndent, *exitCode)))
	}
	return sb.String()
}

const commandSummaryMaxCols = 120
const commandOutputPreviewLines = 3

func renderCommandSummaryLine(prefix string, cmd string, width int) (string, bool) {
	firstLine := cmd
	if i := strings.IndexByte(firstLine, '\n'); i >= 0 {
		firstLine = firstLine[:i] + " …"
	}
	if width <= 0 {
		return prefix + truncateColumns(firstLine, commandSummaryMaxCols), ansi.StringWidth(firstLine) > commandSummaryMaxCols
	}
	prefixWidth := ansi.StringWidth(prefix)
	contentWidth := max(width-prefixWidth, 1)
	maxCols := min(contentWidth, commandSummaryMaxCols)
	truncated := ansi.StringWidth(firstLine) > maxCols
	return prefix + truncateColumns(firstLine, maxCols), truncated
}

type commandDetailsHintParams struct {
	cmdTruncated bool
	moreLines    int
}

func commandDetailsHint(p commandDetailsHintParams) string {
	if p.moreLines > 0 {
		return fmt.Sprintf("%s(+%d more; Ctrl+O history, Ctrl+G open transcript)", agentBodyIndent, p.moreLines)
	}
	if p.cmdTruncated {
		return agentBodyIndent + "(command truncated; Ctrl+O history, Ctrl+G open transcript)"
	}
	return agentBodyIndent + "(Ctrl+O history, Ctrl+G open transcript)"
}

func renderCommandOutputPreview(body string, width int) (string, int) {
	if body == "" {
		return "", 0
	}
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) == 0 {
		return "", 0
	}
	previewCount := min(len(lines), commandOutputPreviewLines)
	out := make([]string, 0, previewCount)
	maxCols := commandSummaryMaxCols
	if width > 0 {
		contentWidth := max(width-ansi.StringWidth(agentBodyIndent), 1)
		maxCols = min(maxCols, contentWidth)
	}
	for i := 0; i < previewCount; i++ {
		out = append(out, agentBodyIndent+truncateColumns(lines[i], maxCols))
	}
	return strings.Join(out, "\n"), len(lines) - previewCount
}

func truncateColumns(s string, maxCols int) string {
	if maxCols <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= maxCols {
		return s
	}
	if maxCols == 1 {
		return "…"
	}
	target := maxCols - 1
	var b strings.Builder
	b.Grow(len(s))
	cols := 0
	for _, r := range s {
		w := ansi.StringWidth(string(r))
		if cols+w > target {
			break
		}
		b.WriteRune(r)
		cols += w
	}
	return b.String() + "…"
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

func wrapLine(line string, width int, prefix string) string {
	if width <= 0 {
		return line
	}
	if prefix == "" {
		return ansi.Wrap(line, width, "")
	}
	// Contract: callers must include the prefix at the start of the line when
	// prefix is non-empty, otherwise the wrapped output cannot preserve prefix
	// alignment.
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
