package room

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestMouseWheel_composeScrollsHistoryAndRearmsLiveAtBottom(t *testing.T) {
	m := seededRoomModel(t)

	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if next.YOffset() >= m.YOffset() {
		t.Fatalf("expected wheel-up to scroll history upward; before=%d after=%d", m.YOffset(), next.YOffset())
	}
	if strings.Contains(roomHeader(next), "LIVE") {
		t.Fatalf("expected wheel-up from compose live to enter browse mode, got %q", roomHeader(next))
	}

	for !next.AtBottom() {
		next, _ = next.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	}
	if !strings.Contains(roomHeader(next), "LIVE") {
		t.Fatalf("expected wheel-down at bottom to re-arm live mode, got %q", roomHeader(next))
	}
}

func TestMouseWheel_historyFocusKeepsCursorVisibleAndReturnsToLiveEnd(t *testing.T) {
	m := seededRoomModel(t)
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))

	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if next.YOffset() >= m.YOffset() {
		t.Fatalf("expected wheel-up to scroll history upward; before=%d after=%d", m.YOffset(), next.YOffset())
	}
	if strings.Contains(roomHeader(next), "LIVE") {
		t.Fatalf("expected wheel-up from history live to enter browse mode, got %q", roomHeader(next))
	}
	if !historyCursorRendered(next) {
		t.Fatal("expected history browse to keep a visible cursor after mouse scrolling")
	}
	if next.history.CursorAtLiveEnd() {
		row, col := next.HistoryCursorPosition()
		t.Fatalf("expected mouse browsing cursor away from live end; row=%d col=%d", row, col)
	}

	for !next.AtBottom() {
		next, _ = next.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	}
	if !strings.Contains(roomHeader(next), "LIVE") {
		t.Fatalf("expected wheel-down at bottom to re-enter history live, got %q", roomHeader(next))
	}
	if !next.history.CursorAtLiveEnd() {
		row, col := next.HistoryCursorPosition()
		t.Fatalf("expected history live cursor at live end; row=%d col=%d", row, col)
	}
}

func TestMouseWheel_historyFocusPreservesVisibleCursorPosition(t *testing.T) {
	m := seededRoomModel(t)
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
	for range 4 {
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	}

	beforeRow, beforeCol := m.HistoryCursorPosition()
	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	afterRow, afterCol := next.HistoryCursorPosition()

	if beforeRow != afterRow || beforeCol != afterCol {
		t.Fatalf("expected wheel scroll to preserve visible history cursor; before=(%d,%d) after=(%d,%d)", beforeRow, beforeCol, afterRow, afterCol)
	}
}
