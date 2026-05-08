package ui

import "strings"

// View renders the current model state as a string for Bubble Tea to display.
func (m Model) View() string {
	if !m.ready {
		return ""
	}
	sep := strings.Repeat("─", m.viewport.Width)
	return m.viewport.View() + "\n" + sep + "\n" + m.input.View()
}
