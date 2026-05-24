// Package history implements the conversation record list and its viewport.
package history

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/trigosec/coderoom/internal/agent"
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
	RecordKindFileChange                    // file patch/diff item from an agent
)

// Record is a single displayable entry in the conversation history.
type Record struct {
	Kind    RecordKind
	Alias   string   // agent alias; empty for user input and system records
	Routing []string // aliases shown in the footer (broadcast / direct send)
	Text    string   // body for non-agent records (user/system/log)
	Msg     *agent.Message
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

func renderRecord(r Record, width int, colors func(string) string) string {
	switch r.Kind {
	case RecordKindUserInput:
		return renderUserInput(r, width, colors)
	case RecordKindAgentOutput:
		return renderAgentOutput(r, width, colors)
	case RecordKindSystem:
		return systemStyle.Render(ansi.Wrap(r.Text, width, ""))
	case RecordKindLog:
		return logStyle.Render(renderLogBody(r.Text, width))
	case RecordKindReasoning:
		return renderReasoning(r, width, colors)
	case RecordKindCommand:
		return renderCommand(r, width, colors)
	case RecordKindFileChange:
		return renderFileChange(r, width, colors)
	}
	return r.Text
}

func renderRecordForTranscript(r Record, colors func(string) string) string {
	if r.Kind == RecordKindCommand {
		return renderCommandTranscript(r, colors)
	}
	if r.Kind == RecordKindFileChange {
		return renderFileChangeTranscript(r, colors)
	}
	// Use width=0 to disable wrapping in transcript exports.
	return renderRecord(r, 0, colors)
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
	plain := promptPrefix + r.Text
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
		// Keep the base text aligned with system messages, and use the
		// participant color only for inline emphasis spans (e.g. **bold**).
		bodyText = inlinefmt.FormatWithStyles(bodyText, systemStyle, lipgloss.NewStyle().Foreground(lipgloss.Color(color)))
	} else {
		bodyText = systemStyle.Render(bodyText)
	}
	wrapped := wrapLine(agentBodyIndent+bodyText, width, agentBodyIndent)
	return header + "\n\n" + wrapped
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

const fileChangePreviewLines = 8

func renderFileChange(r Record, width int, colors func(string) string) string {
	color := colors(r.Alias)
	header := renderParticipantHeader(r.Alias, color)

	var sb strings.Builder
	sb.WriteString(header)

	if isPendingFileChange(r) {
		sb.WriteString("\n\n")
		sb.WriteString(agentBodyIndent + fileChangeBullet + "…")
		return sb.String()
	}

	appendFileChangeList(&sb, r)
	appendFileChangeBodyPreview(&sb, r, width)
	appendFileChangePatchStatus(&sb, r)

	return sb.String()
}

func isPendingFileChange(r Record) bool {
	status, changes, body := fileChangeFieldsFromRecord(r)
	return len(changes) == 0 && body == "" && status == ""
}

func appendFileChangeList(sb *strings.Builder, r Record) {
	_, changes, _ := fileChangeFieldsFromRecord(r)
	if len(changes) == 0 {
		return
	}
	sb.WriteString("\n\n")
	sb.WriteString(agentBodyIndent)
	sb.WriteString(fileChangeBullet)
	sb.WriteString("files:")
	for _, ch := range changes {
		sb.WriteString("\n")
		sb.WriteString(agentBodyIndent)
		sb.WriteString("- ")
		if ch.ChangeKind != "" {
			sb.WriteString(ch.ChangeKind)
			sb.WriteString(" ")
		}
		sb.WriteString(ch.Path)
	}
}

func appendFileChangeBodyPreview(sb *strings.Builder, r Record, width int) {
	_, _, body := fileChangeFieldsFromRecord(r)
	if body == "" {
		return
	}
	sb.WriteString("\n\n")
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	previewCount := min(len(lines), fileChangePreviewLines)
	maxCols := fileChangePreviewMaxCols(width)
	for i := 0; i < previewCount; i++ {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(agentBodyIndent)
		sb.WriteString(truncateColumns(lines[i], maxCols))
	}
	appendFileChangeBodyMoreHint(sb, len(lines)-previewCount)
}

func fileChangePreviewMaxCols(width int) int {
	maxCols := commandSummaryMaxCols
	if width <= 0 {
		return maxCols
	}
	contentWidth := max(width-ansi.StringWidth(agentBodyIndent), 1)
	return min(maxCols, contentWidth)
}

func appendFileChangeBodyMoreHint(sb *strings.Builder, remainingLines int) {
	if remainingLines <= 0 {
		return
	}
	sb.WriteString("\n")
	sb.WriteString(logStyle.Render(fmt.Sprintf(
		"%s(+%d more; Ctrl+O history, Ctrl+G open transcript)",
		agentBodyIndent,
		remainingLines,
	)))
}

func appendFileChangePatchStatus(sb *strings.Builder, r Record) {
	status, _, _ := fileChangeFieldsFromRecord(r)
	if status == "" {
		return
	}
	sb.WriteString("\n")
	sb.WriteString(logStyle.Render(fmt.Sprintf("%s%s", agentBodyIndent, status)))
}

func renderFileChangeTranscript(r Record, colors func(string) string) string {
	color := colors(r.Alias)
	header := renderParticipantHeader(r.Alias, color)
	var sb strings.Builder
	sb.WriteString(header)

	status, changes, body := fileChangeFieldsFromRecord(r)
	if len(changes) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(agentBodyIndent)
		sb.WriteString(fileChangeBullet)
		sb.WriteString("files:")
		for _, ch := range changes {
			sb.WriteString("\n")
			sb.WriteString(agentBodyIndent)
			sb.WriteString("- ")
			if ch.ChangeKind != "" {
				sb.WriteString(ch.ChangeKind)
				sb.WriteString(" ")
			}
			sb.WriteString(ch.Path)
		}
	}

	if body != "" {
		sb.WriteString("\n\n")
		lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
		for i, line := range lines {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(agentBodyIndent + line)
		}
	}

	if status != "" {
		sb.WriteString("\n")
		sb.WriteString(logStyle.Render(fmt.Sprintf("%s%s", agentBodyIndent, status)))
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

func recordKindFor(msg agent.Message) RecordKind {
	switch msg.Content.(type) {
	case agent.Reasoning:
		return RecordKindReasoning
	case agent.Command:
		return RecordKindCommand
	case agent.FileChangeSet:
		return RecordKindFileChange
	}
	return RecordKindAgentOutput
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

func newAgentRecord(alias string, msg agent.Message) Record {
	msgCopy := msg
	return Record{
		Kind:  recordKindFor(msg),
		Alias: alias,
		Msg:   &msgCopy,
		Text:  bodyFrom(msg),
	}
}

func (r Record) accumulate(next agent.Message) (Record, error) {
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
	return r, nil
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

func fileChangeFieldsFromRecord(r Record) (status agent.ToolStatus, changes []agent.FileChange, body string) {
	if r.Msg == nil {
		return "", nil, ""
	}
	c, ok := r.Msg.Content.(agent.FileChangeSet)
	if !ok {
		return "", nil, ""
	}
	if r.Text != "" {
		return c.Status, c.Changes, r.Text
	}
	return c.Status, c.Changes, FormatFileChangeBody(c.Changes)
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
	// Ctrl+O toggles focus (input <-> history). Ctrl+G in history opens a full
	// transcript view in the user's editor.
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
