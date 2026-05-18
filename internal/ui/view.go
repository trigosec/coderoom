package ui

import (
	"strings"
)

// View renders the current model state as a string for Bubble Tea to display.
func (m Model) View() string {
	if !m.history.Ready() {
		return ""
	}
	left := strings.Repeat(" ", marginH)
	sepLabel := m.separatorLabel()
	sep := left + labeledSeparator(m.history.Width(), sepLabel)

	var sb strings.Builder
	for line := range strings.SplitSeq(m.history.View(), "\n") {
		sb.WriteString(left + line + "\n")
	}
	sb.WriteString(sep + "\n")
	for line := range strings.SplitSeq(strings.TrimSuffix(m.compose.View(), "\n"), "\n") {
		sb.WriteString(left + line + "\n")
	}
	for _, line := range strings.Split(m.toolbox.View(), "\n") {
		sb.WriteString(left + line + "\n")
	}
	sb.WriteString(strings.Repeat("\n", marginV))
	// Avoid a trailing newline: when the rendered frame height matches the
	// terminal height, a final newline can scroll the terminal and make the
	// first row appear "missing".
	return strings.TrimRight(sb.String(), "\n")
}

func (m Model) separatorLabel() string {
	label := "compose"
	if m.focus == focusViewport {
		label = "history"
	}
	if !m.debug {
		return label
	}
	return label + " " + m.history.DebugLabel()
}

func labeledSeparator(width int, label string) string {
	if width <= 0 {
		return ""
	}
	mid := " " + label + " "
	if len(mid) >= width {
		return strings.Repeat("─", width)
	}
	leftCount := (width - len(mid)) / 2
	rightCount := width - len(mid) - leftCount
	return strings.Repeat("─", leftCount) + mid + strings.Repeat("─", rightCount)
}
