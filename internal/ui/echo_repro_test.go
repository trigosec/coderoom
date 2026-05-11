package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestWhoEcho_twiceRendersTwoEchosInTallTerminal(t *testing.T) {
	// Reproduce the interactive path (KeyRunes + Enter) rather than calling
	// handleEnter directly.
	m := makeReadyModelWithHeight(t, 40)

	sendLine := func(line string) {
		// Many terminals deliver "normal typing" as KeyRunes with a single rune,
		// but some inputs (IME/paste) may deliver multiple runes at once. Exercise
		// both forms by sending the whole line as one KeyRunes message.
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(line)})
		m = next.(Model)
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(Model)
	}

	sendLine("/who")
	sendLine("/who")

	// Ensure the underlying content contains both echos regardless of scroll.
	content := ansi.Strip(strings.Join(m.renderedRecords, "\n"))
	if strings.Count(content, "❯ /who") != 2 {
		t.Fatalf("expected renderedRecords to contain two echos, got:\n%s", content)
	}

	userInputs := 0
	for _, r := range m.records {
		if r.kind == recordKindUserInput && strings.TrimSpace(r.body) == "/who" {
			userInputs++
		}
	}
	if userInputs != 2 {
		t.Fatalf("expected two echoed user input records, got %d; records=%v", userInputs, m.records)
	}

	// The viewport should stay at the top when all content fits.
	if m.viewport.YOffset != 0 {
		t.Fatalf("expected YOffset=0 in tall terminal, got %d", m.viewport.YOffset)
	}
	view := ansi.Strip(m.viewport.View())
	if strings.Count(view, "❯ /who") != 2 {
		t.Fatalf("expected two visible echos without scrolling; got:\n%s", view)
	}
}

func TestWhoEcho_twiceVisibleInSmallTerminal(t *testing.T) {
	// Regression guard for the "missing first line" symptom in small terminals:
	// `/who` twice should fit without scrolling and show both echos/results.
	m := makeReadyModelWithHeight(t, 10)

	sendLine := func(line string) {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(line)})
		m = next.(Model)
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(Model)
	}

	sendLine("/who")
	sendLine("/who")

	if m.viewport.YOffset != 0 {
		t.Fatalf("expected YOffset=0 when content fits; got %d", m.viewport.YOffset)
	}
	view := ansi.Strip(m.viewport.View())
	if strings.Count(view, "❯ /who") != 2 {
		t.Fatalf("expected two visible echos; got:\n%s", view)
	}
	if strings.Count(view, "[no agents]") != 2 {
		t.Fatalf("expected two visible /who results; got:\n%s", view)
	}
}
