package ui

import "strings"

// View renders the current model state as a string for Bubble Tea to display.
func (m Model) View() string {
	if !m.ready {
		return ""
	}
	left := strings.Repeat(" ", marginH)
	sepLabel := "compose"
	if m.focus == focusViewport {
		sepLabel = "history"
	}
	sep := left + labeledSeparator(m.viewport.Width, sepLabel)

	var sb strings.Builder
	// SplitSeq on an empty string still yields one element, so an empty viewport
	// produces one blank padded line before the separator — acceptable at startup.
	for line := range strings.SplitSeq(strings.TrimSuffix(m.viewport.View(), "\n"), "\n") {
		sb.WriteString(left + line + "\n")
	}
	sb.WriteString(sep + "\n")
	for line := range strings.SplitSeq(strings.TrimSuffix(m.input.View(), "\n"), "\n") {
		sb.WriteString(left + line + "\n")
	}
	sb.WriteString(strings.Repeat("\n", marginV))
	return sb.String()
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
