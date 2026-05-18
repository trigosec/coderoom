package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/ui/room/history"
)

func TestHandleEnter_echoesUserInput(t *testing.T) {
	m := makeReadyModel(t)
	m.compose = m.compose.SetValue("/who")
	m, _ = m.handleSubmit()
	recs := m.history.Records()
	if len(recs) == 0 {
		t.Fatal("expected at least one record after enter")
	}
	if recs[0].Kind != history.RecordKindUserInput {
		t.Errorf("expected first record to be user input, got kind %d", recs[0].Kind)
	}
	if recs[0].Body != "/who" {
		t.Errorf("expected body '/who', got %q", recs[0].Body)
	}
}

func TestEnter_submitsAndClearsInput(t *testing.T) {
	m := makeReadyModel(t)
	m.compose = m.compose.SetValue("hello")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := next.(Model)

	if got := m2.compose.Value(); got != "" {
		t.Fatalf("expected input cleared after submit, got %q", got)
	}
	recs := m2.history.Records()
	if len(recs) == 0 || recs[0].Kind != history.RecordKindUserInput || recs[0].Body != "hello" {
		t.Fatalf("expected first record to echo submitted input; records: %v", recs)
	}
}

func TestEnter_whitespaceOnlyDoesNotCreateRecord(t *testing.T) {
	m := makeReadyModel(t)
	m.compose = m.compose.SetValue("   \n\t ")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := next.(Model)

	if got := m2.compose.Value(); got != "" {
		t.Fatalf("expected input cleared even for whitespace-only submit, got %q", got)
	}
	if len(m2.history.Records()) != 0 {
		t.Fatalf("expected no records for whitespace-only submit, got %d", len(m2.history.Records()))
	}
}

func TestCtrlC_clearsComposerOnlyWhenFocused(t *testing.T) {
	m := makeReadyModel(t)
	m.compose = m.compose.SetValue("draft")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m2 := next.(Model)
	if cmd != nil {
		t.Fatalf("expected Ctrl+C not to quit, got non-nil cmd")
	}
	if got := m2.compose.Value(); got != "" {
		t.Fatalf("expected Ctrl+C to clear composer, got %q", got)
	}

	next, _ = m2.Update(tea.KeyMsg{Type: tea.KeyCtrlO}) // focus viewport
	m3 := next.(Model)
	m3.compose = m3.compose.SetValue("draft2")

	next, cmd = m3.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m4 := next.(Model)
	if cmd != nil {
		t.Fatalf("expected Ctrl+C not to quit in viewport focus, got non-nil cmd")
	}
	if got := m4.compose.Value(); got != "draft2" {
		t.Fatalf("expected Ctrl+C no-op in viewport focus, got %q", got)
	}
}

func TestCtrlO_toggleBackFocusesComposer(t *testing.T) {
	m := makeReadyModel(t)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m2 := next.(Model)
	if m2.focus != focusViewport {
		t.Fatalf("expected focusViewport after first Ctrl+O, got %v", m2.focus)
	}
	if cmd != nil {
		t.Fatal("expected no cmd when blurring textarea on focus switch")
	}

	next, cmd = m2.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m3 := next.(Model)
	if m3.focus != focusComposer {
		t.Fatalf("expected focusComposer after second Ctrl+O, got %v", m3.focus)
	}
	if cmd == nil {
		t.Fatal("expected a cmd from Focusing textarea on focus switch back")
	}
}

func TestInputHeight_isCappedAndDoesNotCollapseViewport(t *testing.T) {
	m := makeReadyModelWithHeight(t, 30) // max input height = min(8, 30/3=10) => 8

	m.compose = m.compose.SetValue(strings.Repeat("x\n", 20) + "x") // 21 lines
	m = m.syncAfterCompose()

	if got := m.compose.Height(); got != 8 {
		t.Fatalf("expected input height capped at 8, got %d", got)
	}
	if m.history.Height() <= 0 {
		t.Fatalf("expected viewport height to stay positive, got %d", m.history.Height())
	}
}

func TestResizeForInput_preservesBottomAnchor(t *testing.T) {
	m := makeReadyModelWithHeight(t, 12)
	for i := range 50 {
		m.history = m.history.AppendSystemRecord("line " + string(rune('0'+i%10)))
	}
	m.history = m.history.GotoBottom()
	if !m.history.AtBottom() {
		t.Fatal("expected to start at bottom")
	}

	m.compose = m.compose.SetValue("a\nb\nc")
	m = m.syncAfterCompose()
	if !m.history.AtBottom() {
		t.Fatal("expected syncAfterCompose to keep viewport anchored to bottom")
	}
}
