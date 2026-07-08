package room

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

func TestHistoryFocus_rendersCursorInViewport(t *testing.T) {
	prev := lipgloss.Writer.Profile
	lipgloss.Writer.Profile = colorprofile.ANSI256
	t.Cleanup(func() { lipgloss.Writer.Profile = prev })

	m := newTestModel(t)
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
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
	lines = strings.Split(next.View(), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines in view; got %d", len(lines))
	}
	if !strings.Contains(lines[1], "\x1b[7m") {
		t.Fatalf("expected history highlight when history is focused; got %q", lines[1])
	}
}
