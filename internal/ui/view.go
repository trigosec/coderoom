package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// View renders the current model state for Bubble Tea to display.
func (m Model) View() tea.View {
	if !m.room.Ready() {
		return tea.NewView("")
	}
	left := strings.Repeat(" ", marginH)

	var sb strings.Builder
	for line := range strings.SplitSeq(m.room.View(), "\n") {
		sb.WriteString(left + line + "\n")
	}
	for _, line := range strings.Split(m.toolbox.View(), "\n") {
		sb.WriteString(left + line + "\n")
	}
	sb.WriteString(strings.Repeat("\n", marginV))
	// Avoid a trailing newline: when the rendered frame height matches the
	// terminal height, a final newline can scroll the terminal and make the
	// first row appear "missing".
	v := tea.NewView(strings.TrimRight(sb.String(), "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
