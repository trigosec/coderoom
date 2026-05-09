package ui

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleEnter_echoesUserInput(t *testing.T) {
	m := makeReadyModel(t)
	m.input.SetValue("/who")
	m, _ = m.handleEnter()
	if len(m.records) == 0 {
		t.Fatal("expected at least one record after enter")
	}
	if m.records[0].kind != recordKindUserInput {
		t.Errorf("expected first record to be user input, got kind %d", m.records[0].kind)
	}
	if m.records[0].body != "/who" {
		t.Errorf("expected body '/who', got %q", m.records[0].body)
	}
}

func TestAltEnter_insertsNewlineWithoutSubmitting(t *testing.T) {
	m := makeReadyModel(t)
	m.input.SetValue("hello")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	m2 := next.(Model)

	if got := m2.input.Value(); got != "hello\n" {
		t.Fatalf("expected Alt+Enter to insert newline, got %q", got)
	}
	if len(m2.records) != 0 {
		t.Fatalf("expected no records (no submit) after Alt+Enter, got %d", len(m2.records))
	}
}

func TestEnter_submitsAndClearsInput(t *testing.T) {
	m := makeReadyModel(t)
	m.input.SetValue("hello")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := next.(Model)

	if got := m2.input.Value(); got != "" {
		t.Fatalf("expected input cleared after submit, got %q", got)
	}
	if len(m2.records) == 0 || m2.records[0].kind != recordKindUserInput || m2.records[0].body != "hello" {
		t.Fatalf("expected first record to echo submitted input; records: %v", m2.records)
	}
}

func TestEnter_whitespaceOnlyDoesNotCreateRecord(t *testing.T) {
	m := makeReadyModel(t)
	m.input.SetValue("   \n\t ")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := next.(Model)

	if got := m2.input.Value(); got != "" {
		t.Fatalf("expected input cleared even for whitespace-only submit, got %q", got)
	}
	if len(m2.records) != 0 {
		t.Fatalf("expected no records for whitespace-only submit, got %d", len(m2.records))
	}
}

func TestInputHeight_isCappedAndDoesNotCollapseViewport(t *testing.T) {
	m := makeReadyModelWithHeight(t, 30) // max input height = min(8, 30/3=10) => 8

	m.input.SetValue(strings.Repeat("x\n", 20) + "x") // 21 lines
	m = m.resizeForInput()

	if got := m.input.Height(); got != 8 {
		t.Fatalf("expected input height capped at 8, got %d", got)
	}
	if m.viewport.Height <= 0 {
		t.Fatalf("expected viewport height to stay positive, got %d", m.viewport.Height)
	}
}

func TestResizeForInput_preservesBottomAnchor(t *testing.T) {
	m := makeReadyModelWithHeight(t, 12)
	for i := range 50 {
		m = m.appendRecord(record{kind: recordKindSystem, body: "line " + strconv.Itoa(i)})
	}
	m.viewport.GotoBottom()
	if !m.viewport.AtBottom() {
		t.Fatal("expected to start at bottom")
	}

	m.input.SetValue("a\nb\nc")
	m = m.resizeForInput()
	if !m.viewport.AtBottom() {
		t.Fatal("expected resizeForInput to keep viewport anchored to bottom")
	}
}
