package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/trigosec/coderoom/internal/session"
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
	beforeView := ansi.Strip(m.viewport.View())

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = next.(Model)
	if m.compose.Value() != beforeInput {
		t.Fatalf("expected PgUp not to mutate composer input; got %q", m.compose.Value())
	}
	afterView := ansi.Strip(m.viewport.View())
	if afterView == beforeView {
		t.Fatalf("expected PgUp to scroll viewport; view unchanged")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = next.(Model)
	if m.compose.Value() != beforeInput {
		t.Fatalf("expected PgDn not to mutate composer input; got %q", m.compose.Value())
	}
}

func TestDelta_whenAtBottom_keepsViewportAtBottom(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)

	// Create enough history to overflow the viewport, then move to bottom.
	for i := 0; i < 25; i++ {
		m = m.appendRecord(record{kind: recordKindSystem, body: "[x]"})
	}
	m.viewport.GotoBottom()
	if !m.viewport.AtBottom() {
		t.Fatalf("expected viewport at bottom before delta")
	}

	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	if !m.viewport.AtBottom() {
		t.Fatalf("expected delta to keep viewport at bottom when already at bottom")
	}
}

func TestDelta_whenScrolledUp_doesNotForceViewportToBottom(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)

	for i := 0; i < 25; i++ {
		m = m.appendRecord(record{kind: recordKindSystem, body: "[x]"})
	}
	m.viewport.GotoBottom()
	m.viewport.ScrollUp(3)
	if m.viewport.AtBottom() {
		t.Fatalf("expected viewport not at bottom after scrolling up")
	}
	y := m.viewport.YOffset

	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	if m.viewport.YOffset != y {
		t.Fatalf("expected delta not to force viewport to bottom when scrolled up; yOffset changed from %d to %d", y, m.viewport.YOffset)
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
	if m.viewport.YOffset != 0 {
		t.Fatalf("expected Home to jump to top; yOffset=%d", m.viewport.YOffset)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = next.(Model)
	if !m.viewport.AtBottom() {
		t.Fatalf("expected End to jump to bottom; yOffset=%d", m.viewport.YOffset)
	}
}
