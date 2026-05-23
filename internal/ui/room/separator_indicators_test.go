package room

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestSeparatorIndicators_noneWhenContentFits(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 20)
	m = m.SetComposeValue("one\ntwo")
	top, bottom := separatorLines(m)
	if strings.Contains(top, "▲") || strings.Contains(top, "▼") {
		t.Errorf("expected no indicators on top separator; got %q", top)
	}
	if strings.Contains(bottom, "▲") || strings.Contains(bottom, "▼") {
		t.Errorf("expected no indicators on bottom separator; got %q", bottom)
	}
}

func TestSeparatorIndicators_aboveWhenOverflowAndCursorAtEnd(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 9) // maxH=3 from totalH=9
	m = m.SetComposeValue("a\nb\nc\nd\ne")
	top, _ := separatorLines(m)
	if !strings.Contains(top, "▲") {
		t.Errorf("expected ▲ on top separator when buffer exceeds viewport; got %q", top)
	}
}

func TestSeparatorIndicators_topNeverShowsDown(t *testing.T) {
	// Even when HasBelow is true, ▼ must only appear on the bottom separator.
	m := New(nil, "")
	m = m.HandleResize(80, 9)
	m = m.SetComposeValue("a\nb\nc\nd\ne")
	for range 10 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	top, _ := separatorLines(m)
	if strings.Contains(top, "▼") {
		t.Errorf("▼ must not appear on the top separator; got %q", top)
	}
}

func TestSeparatorIndicators_belowWhenCursorAtTop(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 9)
	m = m.SetComposeValue("a\nb\nc\nd\ne")
	for range 10 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	_, bottom := separatorLines(m)
	if !strings.Contains(bottom, "▼") {
		t.Errorf("expected ▼ on bottom separator when cursor is at top; got %q", bottom)
	}
}

func TestLabeledSeparator_suffixTrimsRightDashes(t *testing.T) {
	withSuffix := labeledSeparator(20, "x", "▲")
	if len([]rune(ansi.Strip(withSuffix))) != 20 {
		t.Errorf("expected total width 20, got %d: %q", len([]rune(ansi.Strip(withSuffix))), withSuffix)
	}
	if !strings.HasSuffix(withSuffix, "▲") {
		t.Errorf("expected suffix ▲ at right end; got %q", withSuffix)
	}
}

// separatorLines returns the top (compose label) and bottom separator lines from the room view.
func separatorLines(m Model) (top, bottom string) {
	view := ansi.Strip(m.View())
	var seps []string
	for line := range strings.SplitSeq(view, "\n") {
		// The room now includes a 1-line header rendered with dashes; exclude it.
		if strings.HasPrefix(line, "coderoom ") {
			continue
		}
		if strings.Contains(line, "─") {
			seps = append(seps, line)
		}
	}
	if len(seps) >= 1 {
		top = seps[0]
	}
	if len(seps) >= 2 {
		bottom = seps[len(seps)-1]
	}
	return top, bottom
}
