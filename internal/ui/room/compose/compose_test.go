package compose

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestAltEnter_insertsNewlineWithoutSubmitting(t *testing.T) {
	m := New()
	m = m.SetValue("hello")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	if got := m.Value(); got != "hello\n" {
		t.Fatalf("expected Alt+Enter to insert newline, got %q", got)
	}
	if cmd != nil {
		t.Fatal("expected no cmd from Alt+Enter")
	}
}

func TestCtrlC_clearsValue(t *testing.T) {
	m := New()
	m = m.SetValue("draft")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatal("expected no cmd from Ctrl+C")
	}
	if got := m.Value(); got != "" {
		t.Fatalf("expected Ctrl+C to clear, got %q", got)
	}
}

func TestCtrlC_noopWhenEmpty(t *testing.T) {
	m := New()
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatal("expected no cmd from Ctrl+C on empty input")
	}
	if got := m.Value(); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestCtrlRight_movesWordForward(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetValue("hello world")
	// Move cursor to start (Ctrl+A) so there is a word to jump over.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	before := m.input.LineInfo().CharOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	after := m.input.LineInfo().CharOffset
	if after <= before {
		t.Fatalf("expected ctrl+right to advance cursor; before=%d after=%d", before, after)
	}
}

func TestCtrlLeft_movesWordBackward(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetValue("hello world")
	// Cursor starts at end after SetValue; ctrl+left must move it back.
	before := m.input.LineInfo().CharOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlLeft})
	after := m.input.LineInfo().CharOffset
	if after >= before {
		t.Fatalf("expected ctrl+left to move cursor back; before=%d after=%d", before, after)
	}
}

func TestDecorations_shownOnlyForMultiline(t *testing.T) {
	m := New()
	m = m.SetWidth(40)

	if strings.Contains(ansi.Strip(m.View()), "❯   1 ") {
		t.Fatal("expected single-line input to hide prompt line numbers")
	}

	m = m.SetValue("a\nb")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "❯   1 ") || !strings.Contains(view, "❯   2 ") {
		t.Fatalf("expected prompt to show numbers for multiline input, got:\n%s", view)
	}

	m = m.SetValue("a")
	if strings.Contains(ansi.Strip(m.View()), "❯   1 ") {
		t.Fatal("expected prompt line numbers hidden again for single line")
	}
}
