package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestPgUpPgDown_scrollViewportWithoutAffectingComposer(t *testing.T) {
	m := makeReadyModelWithHeight(t, 12)

	// Fill history so PgUp/PgDn have something to scroll.
	for i := 0; i < 30; i++ {
		m.compose = m.compose.SetValue("/who")
		m, _ = m.handleSubmit()
	}

	m.compose = m.compose.SetValue("draft text")
	beforeInput := m.compose.Value()
	beforeView := ansi.Strip(m.history.View())

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = next.(Model)
	if m.compose.Value() != beforeInput {
		t.Fatalf("expected PgUp not to mutate composer input; got %q", m.compose.Value())
	}
	afterView := ansi.Strip(m.history.View())
	if afterView == beforeView {
		t.Fatalf("expected PgUp to scroll viewport; view unchanged")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = next.(Model)
	if m.compose.Value() != beforeInput {
		t.Fatalf("expected PgDn not to mutate composer input; got %q", m.compose.Value())
	}
}

func TestViewportFocus_homeEndJumpToTopBottom(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)
	for i := 0; i < 30; i++ {
		m.compose = m.compose.SetValue("/who")
		m, _ = m.handleSubmit()
	}

	// Enter viewport focus.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m = next.(Model)
	if m.history.YOffset() != 0 {
		t.Fatalf("expected Home to jump to top; yOffset=%d", m.history.YOffset())
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = next.(Model)
	if !m.history.AtBottom() {
		t.Fatalf("expected End to jump to bottom; yOffset=%d", m.history.YOffset())
	}
}

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

	before := ansi.Strip(m2.history.View())
	next, _ = m2.Update(tea.KeyMsg{Type: tea.KeyUp})
	m3 := next.(Model)
	after := ansi.Strip(m3.history.View())

	if after == before {
		t.Fatalf("expected Up to scroll viewport in history focus; view unchanged:\n%s", after)
	}
}
