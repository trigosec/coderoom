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

func TestView_singleLine(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetValue("hello")
	view := ansi.Strip(m.View())
	if !strings.HasPrefix(view, "❯ hello") {
		t.Fatalf("expected single-line view to start with '❯ hello', got:\n%s", view)
	}
}

func TestView_multiLine_continuationIndented(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetMaxHeightFromTotal(20).SetValue("first\nsecond")
	view := ansi.Strip(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 rendered lines, got:\n%s", view)
	}
	if !strings.HasPrefix(lines[0], "❯ ") {
		t.Errorf("first line should start with '❯ ', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  ") {
		t.Errorf("second line should start with '  ', got %q", lines[1])
	}
}

func TestHasAbove_falseWhenContentFits(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetMaxHeightFromTotal(20).SetValue("one\ntwo\nthree")
	if m.HasAbove() {
		t.Error("expected HasAbove=false when all lines fit in the viewport")
	}
	if m.HasBelow() {
		t.Error("expected HasBelow=false when all lines fit in the viewport")
	}
}

func TestHasAbove_trueWhenCursorAtEndOfOverflow(t *testing.T) {
	// maxH from totalH=6 is min(8, max(6/3,1))=2. Set enough lines to overflow.
	m := New()
	m = m.SetWidth(40).SetMaxHeightFromTotal(6).SetValue("a\nb\nc\nd\ne")
	// Cursor is at the last line after SetValue; content above must exist.
	if !m.HasAbove() {
		t.Error("expected HasAbove=true when buffer exceeds viewport height")
	}
}

func TestHasBelow_trueWhenCursorAtTop(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetMaxHeightFromTotal(6).SetValue("a\nb\nc\nd\ne")
	// Move cursor to the first line via Ctrl+Home equivalent (Ctrl+A then Up).
	for range 10 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if !m.HasBelow() {
		t.Error("expected HasBelow=true when cursor is at the top of an overflowing buffer")
	}
}

func TestView_longLine_wrapsWithIndent(t *testing.T) {
	m := New()
	// Width 12: prompt takes 2, content area is 10.
	// "0123456789X" is 11 runes — should wrap onto a second visual row.
	m = m.SetWidth(12).SetMaxHeightFromTotal(20).SetValue("0123456789X")
	view := ansi.Strip(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapped long line to produce >=2 rendered rows, got:\n%s", view)
	}
	if !strings.HasPrefix(lines[0], "❯ ") {
		t.Errorf("first visual row should start with '❯ ', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  ") {
		t.Errorf("continuation row should start with '  ', got %q", lines[1])
	}
}
