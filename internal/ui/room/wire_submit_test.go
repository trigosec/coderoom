package room

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEnter_emitsSubmitMsgAndClearsComposer(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 20)
	m = m.SetComposeValue("hello")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := next.ComposeValue(); got != "" {
		t.Fatalf("expected Enter to clear composer, got %q", got)
	}
	if cmd == nil {
		t.Fatal("expected a SubmitMsg command from Enter")
	}
	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", msg)
	}
	if submit.Text != "hello" {
		t.Fatalf("expected SubmitMsg text %q, got %q", "hello", submit.Text)
	}
}

func TestEnter_whitespaceOnlyDoesNotEmitSubmitMsg(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 20)
	m = m.SetComposeValue("   \n\t ")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := next.ComposeValue(); got != "" {
		t.Fatalf("expected Enter to clear composer even for whitespace-only input, got %q", got)
	}
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected no SubmitMsg for whitespace-only input, got %T (%v)", msg, msg)
	}
}

func TestCtrlC_clearsComposerOnlyWhenFocused(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 20)
	m = m.SetComposeValue("draft")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatalf("expected Ctrl+C to return nil cmd, got non-nil")
	}
	if got := next.ComposeValue(); got != "" {
		t.Fatalf("expected Ctrl+C to clear composer, got %q", got)
	}

	// Switch to history focus.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	next = next.SetComposeValue("draft2")

	next2, _ := next.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if got := next2.ComposeValue(); got != "draft2" {
		t.Fatalf("expected Ctrl+C no-op in history focus, got %q", got)
	}
}

func TestComposerHeight_isCappedAndDoesNotCollapseHistory(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 30) // max input height = min(8, 30/3=10) => 8

	m = m.SetComposeValue(strings.Repeat("x\n", 20) + "x") // 21 lines
	if got := m.ComposeHeight(); got != 8 {
		t.Fatalf("expected compose height capped at 8, got %d", got)
	}
	if m.HistoryHeight() <= 0 {
		t.Fatalf("expected history height to stay positive, got %d", m.HistoryHeight())
	}
}

func TestComposeResize_preservesBottomAnchor(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 12)
	for range 50 {
		m = m.AppendSystem("line")
	}
	m = m.GotoBottom()
	if !m.AtBottom() {
		t.Fatal("expected to start at bottom")
	}

	m = m.SetComposeValue("a\nb\nc")
	if !m.AtBottom() {
		t.Fatal("expected compose resize to keep history anchored to bottom")
	}
}
