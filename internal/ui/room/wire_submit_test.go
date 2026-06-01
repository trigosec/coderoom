package room

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/ui/room/staging"
)

func assertCmdContainsStagedEditMsg(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected StagedEditMsg cmd, got nil")
	}
	msg := cmd()
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			if c == nil {
				continue
			}
			if _, ok := c().(StagedEditMsg); ok {
				return
			}
		}
		t.Fatalf("expected StagedEditMsg in batch, got %T (%v)", msg, msg)
	case StagedEditMsg:
		return
	default:
		t.Fatalf("expected StagedEditMsg, got %T", msg)
	}
}

func TestEnter_emitsSubmitMsgAndClearsComposer(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 20)
	m = m.SetComposeValue("hello")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := next.ComposeValue(); got != "" {
		t.Fatalf("expected Enter to clear composer immediately, got %q", got)
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

func TestEnter_secondPressDoesNotEmitDuplicateSubmit(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 20)
	m = m.SetComposeValue("hello")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected first Enter to emit SubmitMsg")
	}
	next2, cmd2 := next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd2 != nil {
		msg := cmd2()
		t.Fatalf("expected second Enter to emit no command, got %T (%v)", msg, msg)
	}
	if got := next2.ComposeValue(); got != "" {
		t.Fatalf("expected composer to stay cleared after second Enter, got %q", got)
	}
}

func TestEnter_whitespaceOnlyDoesNotEmitSubmitMsg(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 20)
	m = m.SetComposeValue("   \n\t ")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := next.ComposeValue(); got == "" {
		t.Fatalf("expected Enter not to clear composer for whitespace-only input")
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

func TestStagedStatusResize_preservesBottomAnchor(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 12)
	for range 50 {
		m = m.AppendSystem("line")
	}
	m = m.GotoBottom()
	if !m.AtBottom() {
		t.Fatal("expected to start at bottom")
	}

	b := staging.NewBatch(
		"/send a hi",
		staging.Action{Kind: staging.ActionSend, Alias: "a", Text: "hi"},
		[]string{"a"},
	)
	m = m.StageBatch(b, []string{"a"}) // blocked => staged status line adds a row
	if !m.AtBottom() {
		t.Fatal("expected staged status line resize to keep history anchored to bottom")
	}
}

func TestStagedComposer_blocksKeysAndEscEmitsEditMsg(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 20)
	m = m.SetComposerStaged("hello", "Message on-hold.")

	// Random typing is ignored.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd != nil {
		t.Fatalf("expected no cmd from typing while staged, got non-nil")
	}
	if got := next.ComposeValue(); got != "hello" {
		t.Fatalf("expected staged text unchanged, got %q", got)
	}

	// Esc exits staged mode and emits StagedEditMsg.
	next2, cmd2 := next.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if next2.IsComposerStaged() {
		t.Fatal("expected staged mode cleared after Esc")
	}
	assertCmdContainsStagedEditMsg(t, cmd2)
	if got := next2.ComposeValue(); got != "hello" {
		t.Fatalf("expected draft to preserve staged text for editing, got %q", got)
	}
}

func TestDispatchStagedBatch_restoresComposerFocus(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 20)

	b := staging.NewBatch(
		"/send a hi",
		staging.Action{Kind: staging.ActionSend, Alias: "a", Text: "hi"},
		[]string{"a"},
	)
	m = m.StageBatch(b, nil)

	next, _, _ := m.DispatchStagedBatch()
	if next.IsComposerStaged() {
		t.Fatal("expected staged mode cleared after dispatch")
	}

	// Typing should work again after auto-dispatch clears staging.
	next2, _ := next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if got := next2.ComposeValue(); got != "x" {
		t.Fatalf("expected composer to accept input after dispatch, got %q", got)
	}
}
