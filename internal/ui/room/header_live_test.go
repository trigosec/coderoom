package room

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestHeaderLive_whenBottomAlignedButCursorLeavesLiveEnd_hidesLive(t *testing.T) {
	m := newTestModel(t)
	m = m.HandleResize(40, 10)
	for range 30 {
		m = m.AppendSystem("line")
	}
	m = m.GoLive()

	if !strings.Contains(roomHeader(m), "LIVE") {
		t.Fatal("expected bottom-aligned composer view to start live")
	}

	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
	if !m.AtBottom() {
		t.Fatalf("expected Home on the last visible row to keep viewport bottom-aligned; yOffset=%d", m.YOffset())
	}

	header := roomHeader(m)
	if strings.Contains(header, "LIVE") {
		t.Fatalf("expected LIVE to disappear when cursor leaves live end at bottom; got %q", header)
	}
	if !strings.Contains(header, "(PgDn:") {
		t.Fatalf("expected non-live header to advertise PgDn; got %q", header)
	}
}

func TestHeaderLive_whenComposerBrowseReturnsToBottom_rearmsLive(t *testing.T) {
	m := newTestModel(t)
	m = m.HandleResize(40, 10)
	for range 30 {
		m = m.AppendSystem("line")
	}
	m = m.GoLive()

	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyHome}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	for !m.AtBottom() {
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	}

	header := roomHeader(m)
	if !strings.Contains(header, "LIVE") {
		t.Fatalf("expected returning to bottom in composer focus to re-arm LIVE; got %q", header)
	}

	beforeY := m.YOffset()
	m = m.AppendSystem("[new]")
	if !m.AtBottom() {
		t.Fatalf("expected live composer view to follow new output; yOffset=%d", m.YOffset())
	}
	if m.YOffset() < beforeY {
		t.Fatalf("expected new output not to move viewport upward; before=%d after=%d", beforeY, m.YOffset())
	}
}

func TestHeaderLive_whenComposerBrowseReceivesOutput_staysBrowsing(t *testing.T) {
	m := newTestModel(t)
	m = m.HandleResize(40, 10)
	for range 30 {
		m = m.AppendSystem("line")
	}
	m = m.GoLive()

	beforeBrowseY := m.YOffset()
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	if m.YOffset() == beforeBrowseY {
		t.Fatalf("expected PgUp to enter composer browse mode; yOffset unchanged (%d)", beforeBrowseY)
	}
	browseY := m.YOffset()
	if strings.Contains(roomHeader(m), "LIVE") {
		t.Fatalf("expected composer browse header not to show LIVE, got %q", roomHeader(m))
	}

	m = m.AppendSystem("[new]")
	if m.YOffset() != browseY {
		t.Fatalf("expected composer browse to preserve viewport on new output; before=%d after=%d", browseY, m.YOffset())
	}
	if strings.Contains(roomHeader(m), "LIVE") {
		t.Fatalf("expected composer browse to stay non-live after new output, got %q", roomHeader(m))
	}
}

func roomHeader(m Model) string {
	view := ansi.Strip(m.View())
	line, _, _ := strings.Cut(view, "\n")
	return line
}
