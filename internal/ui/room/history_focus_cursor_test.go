package room

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestHistoryFocus_rendersCursorInViewport(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := New(nil, "")
	m = m.HandleResize(40, 10)
	m = m.AppendSystem("hello")

	// Default focus is compose/input: no history focus highlight.
	lines := strings.Split(m.View(), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines in view; got %d", len(lines))
	}
	if strings.Contains(lines[1], "\x1b[7m") {
		t.Fatalf("did not expect history highlight when compose is focused; got %q", lines[1])
	}

	// Enter history focus.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	lines = strings.Split(next.View(), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines in view; got %d", len(lines))
	}
	if !strings.Contains(lines[1], "\x1b[7m") {
		t.Fatalf("expected history highlight when history is focused; got %q", lines[1])
	}
}
