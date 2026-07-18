package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestView_enablesMouseWheelMode(t *testing.T) {
	m := makeReadyModel(t)

	view := m.View()
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("expected mouse cell-motion mode, got %v", view.MouseMode)
	}
	if !view.AltScreen {
		t.Fatal("expected alt-screen view")
	}
}
