package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestHistoryFocus_arrowKeysScrollViewport(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)

	// Create enough records to overflow the viewport height.
	for i := 0; i < 10; i++ {
		m.compose = m.compose.SetValue("/who")
		m, _ = m.handleSubmit()
	}

	// Enter history focus.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m2 := next.(Model)

	before := ansi.Strip(m2.viewport.View())
	next, _ = m2.Update(tea.KeyMsg{Type: tea.KeyUp})
	m3 := next.(Model)
	after := ansi.Strip(m3.viewport.View())

	if after == before {
		t.Fatalf("expected Up to scroll viewport in history focus; view unchanged:\n%s", after)
	}
}
