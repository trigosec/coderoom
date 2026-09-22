package ui

import (
	"strings"
	"testing"

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

func countRecords(m Model, kind record.Kind, text string) int {
	count := 0
	for _, item := range m.room.HistoryRecords() {
		if item.Kind == kind && strings.TrimSpace(item.Text) == text {
			count++
		}
	}
	return count
}
