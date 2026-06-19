package history

import (
	"testing"

	rec "github.com/trigosec/coderoom/internal/ui/room/history/record"
)

func systemRecords(n int) []rec.Record {
	records := make([]rec.Record, n)
	for i := range records {
		records[i] = rec.Record{Kind: rec.KindSystem, Text: "[x]"}
	}
	return records
}

func TestReplaceState_whenAtBottom_keepsViewportAtBottom(t *testing.T) {
	m := New(nil, "")
	m = m.SetSize(80, 10)
	records := systemRecords(25)
	m = m.ReplaceState(State{Records: records})
	m = m.GotoBottom()
	if !m.AtBottom() {
		t.Fatal("expected viewport at bottom before new state")
	}

	records = append(records, rec.Record{Kind: rec.KindAgentOutput, Alias: "ada", Text: "hello"})
	m = m.ReplaceState(State{Records: records})
	if !m.AtBottom() {
		t.Fatal("expected new state to keep viewport at bottom when already at bottom")
	}
}

func TestReplaceState_whenScrolledUp_doesNotForceViewportToBottom(t *testing.T) {
	m := New(nil, "")
	m = m.SetSize(80, 10)
	records := systemRecords(25)
	m = m.ReplaceState(State{Records: records})
	m = m.GotoBottom()
	m = m.ScrollUp(3)
	if m.AtBottom() {
		t.Fatal("expected viewport not at bottom after scrolling up")
	}
	y := m.YOffset()

	records = append(records, rec.Record{Kind: rec.KindAgentOutput, Alias: "ada", Text: "hello"})
	m = m.ReplaceState(State{Records: records})
	if m.YOffset() != y {
		t.Fatalf("expected new state not to force viewport to bottom when scrolled up; yOffset changed from %d to %d", y, m.YOffset())
	}
}
