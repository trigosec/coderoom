package ui

import (
	"strings"
	"testing"
)

func TestView_doesNotEndWithNewline(t *testing.T) {
	m := makeReadyModelWithHeight(t, 48)

	// Add some content so we exercise the viewport/input/toolbox composition.
	m.room = m.room.AppendSystem("[x]")

	view := m.View().Content
	if strings.HasSuffix(view, "\n") {
		// Bug/regression guard:
		// When the rendered frame height matches the terminal height, a trailing
		// newline can scroll the terminal by 1 row. In practice this made the
		// first viewport line appear "missing" (e.g. after `/who` twice, the
		// screen started at the second record) even though viewport.YOffset=0 and
		// viewport.View() contained the correct top-of-history line.
		t.Fatalf("expected View() not to end with newline; trailing newline can scroll terminal and hide the first row")
	}
}

func TestView_doesNotExceedTerminalHeight(t *testing.T) {
	const height = 30
	m := makeReadyModelWithHeight(t, height)

	// Add a few records so the viewport isn't empty.
	for i := 0; i < 3; i++ {
		m.room = m.room.AppendSystem("[x]")
	}

	view := m.View().Content
	lines := 0
	if view != "" {
		lines = strings.Count(view, "\n") + 1
	}
	if lines > height {
		t.Fatalf("expected View() to render at most terminal height=%d lines, got %d", height, lines)
	}
}
