package room

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPgUpPgDown_scrollHistoryWithoutAffectingComposer(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 12)

	for range 40 {
		m = m.AppendSystem("[x]")
	}
	m = m.GotoBottom()

	m = m.SetComposeValue("draft text")
	beforeInput := m.ComposeValue()
	beforeY := m.YOffset()

	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	if next.ComposeValue() != beforeInput {
		t.Fatalf("expected PgUp not to mutate composer input; got %q", next.ComposeValue())
	}
	if next.YOffset() == beforeY {
		t.Fatalf("expected PgUp to scroll history; yOffset unchanged (%d)", beforeY)
	}

	next2, _ := next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	if next2.ComposeValue() != beforeInput {
		t.Fatalf("expected PgDn not to mutate composer input; got %q", next2.ComposeValue())
	}
}

func TestHistoryFocus_homeEndJumpToTopBottom(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 10)
	for range 40 {
		m = m.AppendSystem("[x]")
	}

	// Enter history focus.
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))

	next, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
	if next.YOffset() != 0 {
		t.Fatalf("expected Home to jump to top; yOffset=%d", next.YOffset())
	}

	next, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
	if !next.AtBottom() {
		t.Fatalf("expected End to jump to bottom; yOffset=%d", next.YOffset())
	}
}

func TestHistoryFocus_arrowKeysScroll(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 10)
	for range 40 {
		m = m.AppendSystem("[x]")
	}
	m = m.GotoBottom()

	// Enter history focus.
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
	beforeY := next.YOffset()

	next2, _ := next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if next2.YOffset() == beforeY {
		t.Fatalf("expected Up to scroll history in history focus; yOffset unchanged (%d)", beforeY)
	}
}
