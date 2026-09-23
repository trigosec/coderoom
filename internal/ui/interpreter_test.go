package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/trigosec/coderoom/internal/interpreter"
	"github.com/trigosec/coderoom/internal/ui/room/history/record"
)

func TestSubmit_UnknownInterpreterCommandFallsBackToLegacyDispatcher(t *testing.T) {
	m := makeReadyModel(t)

	m = submitThroughInterpreter(t, m, "/who")

	if got := countRecords(m, record.KindUserInput, "/who"); got != 1 {
		t.Fatalf("user input records = %d, want 1", got)
	}
	if !hasRecord(m, record.KindSystem, "[no agents]") {
		t.Fatalf("expected legacy /who result; records: %v", m.room.HistoryRecords())
	}
}

func TestSubmit_InvalidInputIsRenderedWithoutLegacyFallback(t *testing.T) {
	m := makeReadyModel(t)

	m = submitThroughInterpreter(t, m, "/invite")

	if got := countRecords(m, record.KindUserInput, "/invite"); got != 0 {
		t.Fatalf("user input records = %d, want 0", got)
	}
	if !hasRecord(m, record.KindSystem, "error:") {
		t.Fatalf("expected interpreter rejection; records: %v", m.room.HistoryRecords())
	}
}

func TestSubmit_ResponseDoesNotClearNewComposerDraft(t *testing.T) {
	m := makeReadyModel(t)
	m.room = m.room.SetComposeValue("/who")

	m = m.submitToInterpreter("/who")
	if got := m.room.ComposeValue(); got != "" {
		t.Fatalf("composer after enqueue = %q, want empty", got)
	}
	m.room = m.room.SetComposeValue("next draft")
	m = processInterpreterSubmission(t, m)

	if got := m.room.ComposeValue(); got != "next draft" {
		t.Fatalf("composer after response = %q, want new draft preserved", got)
	}
}

func TestSubmit_GatesSecondSubmissionUntilTerminalOutcome(t *testing.T) {
	m := makeReadyModel(t)
	m.room = m.room.SetComposeValue("/who")
	first, firstCmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = first.(Model)
	if !m.submissionPending {
		t.Fatal("submission gate is not active before SubmitMsg delivery")
	}
	if firstCmd == nil {
		t.Fatal("first Enter did not return a submission command")
	}

	m.room = m.room.SetComposeValue("/help")
	second, secondCmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = second.(Model)
	if secondCmd != nil {
		t.Fatal("second Enter returned a command while submission was pending")
	}
	if got := m.room.ComposeValue(); got != "/help" {
		t.Fatalf("composer while gated = %q, want preserved draft", got)
	}

	firstMsg := firstCmd()
	first, _ = m.Update(firstMsg)
	m = first.(Model)
	m = processInterpreterSubmission(t, m)
	if m.submissionPending {
		t.Fatal("submission gate remained active after terminal outcome")
	}
	if got := m.room.ComposeValue(); got != "/help" {
		t.Fatalf("composer after terminal outcome = %q, want preserved draft", got)
	}
	if got := countRecords(m, record.KindUserInput, "/help"); got != 0 {
		t.Fatalf("second submission records = %d, want 0", got)
	}
}

func TestSubmit_TerminalOutcomesReleaseGateWithoutClearingDraft(t *testing.T) {
	tests := []struct {
		name  string
		event interpreter.Event
	}{
		{name: "rejected", event: interpreter.InputRejected{Raw: "/invite", Err: errors.New("rejected")}},
		{name: "unknown", event: interpreter.UnknownCommand{Raw: "/who", Name: "who"}},
		{name: "succeeded", event: interpreter.SubmissionSucceeded{Raw: "/invite ada"}},
		{name: "failed", event: interpreter.SubmissionFailed{Raw: "/invite ada", Operation: "invite", Err: errors.New("failed")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := makeReadyModel(t)
			m.submissionPending = true
			m.room = m.room.SetComposeValue("next draft")

			m, _ = m.handleInterpreterEvent(tt.event)

			if m.submissionPending {
				t.Fatal("submission gate remained active")
			}
			if got := m.room.ComposeValue(); got != "next draft" {
				t.Fatalf("composer = %q, want preserved draft", got)
			}
		})
	}
}

func TestSubmit_ShutdownRejectionRestoresClearedDraft(t *testing.T) {
	m := makeReadyModel(t)
	m.interpreter.Close()

	m = m.submitToInterpreter("/who")

	if m.submissionPending {
		t.Fatal("submission gate activated after enqueue rejection")
	}
	if got := m.room.ComposeValue(); got != "/who" {
		t.Fatalf("composer = %q, want restored submission", got)
	}
}

func countRecords(m Model, kind record.Kind, text string) int {
	count := 0
	for _, item := range m.room.HistoryRecords() {
		if item.Kind == kind && strings.TrimSpace(item.Text) == text {
			count++
		}
	}
	return count
}
