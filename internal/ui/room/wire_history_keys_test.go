package room

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPgUpPgDown_scrollHistoryWithoutAffectingComposer(t *testing.T) {
	m := newTestModel(t)
	m = m.HandleResize(80, 12)

	for range 40 {
		m = m.AppendSystem("[x]")
	}
	m = m.GoLive()

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

func TestHistoryFocus_preservesComposerScrollWhenReturningToHistory(t *testing.T) {
	m := newTestModel(t)
	m = m.HandleResize(20, 10)
	for range 30 {
		m = m.AppendSystem("line")
	}

	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
	cursorRow, _ := next.HistoryCursorPosition()

	next, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	scrolled, _ := next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	if scrolled.YOffset() >= cursorRow {
		t.Fatalf("expected composer PgUp to scroll cursor off-screen; yOffset=%d cursorRow=%d", scrolled.YOffset(), cursorRow)
	}
	beforeRefocusY := scrolled.YOffset()

	refocused, _ := scrolled.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
	if refocused.YOffset() != beforeRefocusY {
		t.Fatalf("expected returning to history focus to preserve viewport; before=%d after=%d", beforeRefocusY, refocused.YOffset())
	}
	refocusedRow, _ := refocused.HistoryCursorPosition()
	if refocusedRow < refocused.YOffset() || refocusedRow >= refocused.YOffset()+refocused.HistoryHeight() {
		t.Fatalf("expected returning to history focus to place cursor in visible viewport; row=%d yOffset=%d height=%d", refocusedRow, refocused.YOffset(), refocused.HistoryHeight())
	}
}

func TestHistoryFocus_homeEndJumpToTopBottom(t *testing.T) {
	m := newTestModel(t)
	m = m.HandleResize(12, 10)
	m = m.AppendSystem("hello world")

	// Enter history focus.
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
	row, col := next.HistoryCursorPosition()
	if row == 0 && col == 0 {
		t.Fatalf("expected cursor to start at live end, got row=%d col=%d", row, col)
	}

	next, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
	row, col = next.HistoryCursorPosition()
	if row != 0 || col != 0 {
		t.Fatalf("expected Home to move to visible line start; got row=%d col=%d", row, col)
	}

	next, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
	row, col = next.HistoryCursorPosition()
	if row != 0 || col == 0 {
		t.Fatalf("expected End to move to visible line end; got row=%d col=%d", row, col)
	}
}

func TestHistoryFocus_arrowKeysMoveCursorAndScroll(t *testing.T) {
	m := newTestModel(t)
	m = m.HandleResize(80, 10)
	for range 40 {
		m = m.AppendSystem("[x]")
	}
	m = m.GoLive()

	// Enter history focus.
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
	beforeY := next.YOffset()
	beforeRow, _ := next.HistoryCursorPosition()

	next2, _ := next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	afterRow, _ := next2.HistoryCursorPosition()
	if afterRow >= beforeRow {
		t.Fatalf("expected Up to move the cursor upward; before=%d after=%d", beforeRow, afterRow)
	}
	if next2.YOffset() == beforeY && beforeRow == afterRow {
		t.Fatalf("expected Up to move cursor and/or scroll history; row=%d yOffset=%d", afterRow, next2.YOffset())
	}
}

func TestHistoryFocus_resizePreservesExistingCursor(t *testing.T) {
	m := newTestModel(t)
	m = m.HandleResize(20, 12)
	for range 30 {
		m = m.AppendSystem("line")
	}

	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	beforeRow, beforeCol := m.HistoryCursorPosition()
	if m.AtBottom() && beforeRow == 0 && beforeCol == 0 {
		t.Fatalf("expected cursor to move away from live end before resize; row=%d col=%d", beforeRow, beforeCol)
	}

	resized := m.HandleResize(20, 11)
	afterRow, afterCol := resized.HistoryCursorPosition()
	if afterRow != beforeRow || afterCol != beforeCol {
		t.Fatalf("expected resize to preserve history cursor; before=(%d,%d) after=(%d,%d)", beforeRow, beforeCol, afterRow, afterCol)
	}
}
