package ui

import (
	"strconv"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/session"
)

func TestPgDn_scrollsViewportAndDoesNotAffectInput(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)
	m.input.SetValue("draft")

	for i := range 40 {
		m = m.appendRecord(record{kind: recordKindSystem, body: "line " + strconv.Itoa(i)})
	}
	m.viewport.GotoTop()
	start := m.viewport.YOffset

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m2 := next.(Model)

	if m2.viewport.YOffset <= start {
		t.Fatalf("expected PgDn to scroll viewport down; before=%d after=%d", start, m2.viewport.YOffset)
	}
	if got := m2.input.Value(); got != "draft" {
		t.Fatalf("expected PgDn not to change input, got %q", got)
	}
}

func TestPgUp_scrollsViewportUpAndDoesNotAffectInput(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)
	m.input.SetValue("draft")

	for i := range 40 {
		m = m.appendRecord(record{kind: recordKindSystem, body: "line " + strconv.Itoa(i)})
	}
	m.viewport.GotoBottom()
	start := m.viewport.YOffset

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m2 := next.(Model)

	if m2.viewport.YOffset >= start {
		t.Fatalf("expected PgUp to scroll viewport up; before=%d after=%d", start, m2.viewport.YOffset)
	}
	if got := m2.input.Value(); got != "draft" {
		t.Fatalf("expected PgUp not to change input, got %q", got)
	}
}

func TestDelta_whenAtBottom_keepsViewportAtBottom(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)
	for i := range 40 {
		m = m.appendRecord(record{kind: recordKindSystem, body: "line " + strconv.Itoa(i)})
	}
	m.viewport.GotoBottom()
	if !m.viewport.AtBottom() {
		t.Fatal("expected to start at bottom")
	}

	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	if !m.viewport.AtBottom() {
		t.Fatal("expected viewport to remain at bottom after delta when already at bottom")
	}
}

func TestDelta_whenScrolledUp_doesNotForceViewportToBottom(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)
	for i := range 40 {
		m = m.appendRecord(record{kind: recordKindSystem, body: "line " + strconv.Itoa(i)})
	}
	m.viewport.GotoTop()
	if m.viewport.AtBottom() {
		t.Fatal("expected not to be at bottom when positioned at top")
	}
	start := m.viewport.YOffset

	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	if m.viewport.AtBottom() {
		t.Fatal("expected viewport not to jump to bottom when user is scrolled up")
	}
	if m.viewport.YOffset != start {
		t.Fatalf("expected viewport y-offset unchanged when scrolled up; before=%d after=%d", start, m.viewport.YOffset)
	}
}

func TestViewportFocus_arrowsScrollViewport(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)
	for i := range 40 {
		m = m.appendRecord(record{kind: recordKindSystem, body: "line " + strconv.Itoa(i)})
	}
	m.viewport.GotoTop()
	start := m.viewport.YOffset

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO}) // focus viewport
	m2 := next.(Model)
	if m2.focus != focusViewport {
		t.Fatalf("expected focusViewport after Ctrl+O, got %v", m2.focus)
	}

	next, _ = m2.Update(tea.KeyMsg{Type: tea.KeyDown})
	m3 := next.(Model)
	if m3.viewport.YOffset <= start {
		t.Fatalf("expected Down to scroll viewport in viewport focus; before=%d after=%d", start, m3.viewport.YOffset)
	}
}

func TestViewportFocus_homeEndJumpToTopBottom(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)
	for i := range 80 {
		m = m.appendRecord(record{kind: recordKindSystem, body: "line " + strconv.Itoa(i)})
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO}) // focus viewport
	m2 := next.(Model)

	next, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m3 := next.(Model)
	if !m3.viewport.AtBottom() {
		t.Fatal("expected End to jump to bottom in viewport focus")
	}

	next, _ = m3.Update(tea.KeyMsg{Type: tea.KeyHome})
	m4 := next.(Model)
	if m4.viewport.YOffset != 0 {
		t.Fatalf("expected Home to jump to top in viewport focus, got y=%d", m4.viewport.YOffset)
	}
}
