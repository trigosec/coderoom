package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/trigosec/coderoom/internal/participant"
)

func TestToolboxCells_orderByStatusThenAlias(t *testing.T) {
	m := makeReadyModel(t)
	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }

	ps := []participant.Participant{
		{Alias: "bob", Status: participant.StatusIdle, Since: now},
		{Alias: "ada", Status: participant.StatusWorking, Since: now.Add(-5 * time.Second)},
		{Alias: "zoe", Status: participant.StatusWorking, Since: now.Add(-7 * time.Second)},
		{Alias: "cat", Status: participant.StatusStarting, Since: now.Add(-2 * time.Second)},
	}

	out := m.renderParticipantCells(cellWidth()*10, now, ps)
	// working first (alias-sorted within tier): ada, zoe; then starting: cat; then idle: bob
	wantOrder := []string{"ada", "zoe", "cat", "bob"}
	pos := -1
	for _, alias := range wantOrder {
		i := indexAfter(out, alias, pos)
		if i < 0 {
			t.Fatalf("expected %q in output; got: %q", alias, out)
		}
		pos = i
	}
}

func TestToolboxCells_overflowShowsPlusN(t *testing.T) {
	m := makeReadyModel(t)
	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }

	ps := make([]participant.Participant, 0, 10)
	for i := 0; i < 10; i++ {
		ps = append(ps, participant.Participant{
			Alias:  string(rune('a' + i)),
			Status: participant.StatusIdle,
			Since:  now,
		})
	}
	// Force only 2 cells wide -> 1 visible + overflow cell.
	out := m.renderParticipantCells(cellWidth()*2, now, ps)
	if !contains(out, "+9") {
		t.Fatalf("expected overflow +9, got: %q", out)
	}
}

func TestToolboxGlyphs_andElapsedFormatting(t *testing.T) {
	m := makeReadyModel(t)
	base := time.Unix(100, 0)
	m.now = func() time.Time { return base }

	ps := []participant.Participant{
		{Alias: "ada", Status: participant.StatusWorking, Since: base.Add(-10 * time.Second)},
	}
	out := m.renderParticipantCells(cellWidth()*4, base, ps)
	if !contains(out, "⏹") && !contains(out, "◆") {
		t.Fatalf("expected working glyph in output, got: %q", out)
	}
	if !contains(out, "(10s)") {
		t.Fatalf("expected 10s elapsed in output, got: %q", out)
	}

	ps = append(ps, participant.Participant{
		Alias:  "bob",
		Status: participant.StatusCrashed,
		Since:  base.Add(-3*time.Minute - 12*time.Second),
	})
	out = m.renderParticipantCells(cellWidth()*4, base, ps)
	if !contains(out, "✖") {
		t.Fatalf("expected crashed glyph in output, got: %q", out)
	}
	if !contains(out, "(3m12s)") {
		t.Fatalf("expected 3m12s elapsed in output, got: %q", out)
	}
}

func TestRosterWantsTick(t *testing.T) {
	now := time.Unix(100, 0)
	if rosterWantsTick(nil) {
		t.Fatal("expected false for empty roster")
	}
	if rosterWantsTick([]participant.Participant{{Alias: "ada", Status: participant.StatusIdle, Since: now}}) {
		t.Fatal("expected false for idle-only roster")
	}
	if !rosterWantsTick([]participant.Participant{{Alias: "ada", Status: participant.StatusWorking, Since: now}}) {
		t.Fatal("expected true for working participant")
	}
	if !rosterWantsTick([]participant.Participant{{Alias: "ada", Status: participant.StatusStarting, Since: now}}) {
		t.Fatal("expected true for starting participant")
	}
	if !rosterWantsTick([]participant.Participant{{Alias: "ada", Status: participant.StatusCrashed, Since: now}}) {
		t.Fatal("expected true for crashed participant")
	}
}

func indexAfter(s, sub string, after int) int {
	start := 0
	if after >= 0 {
		start = min(after+1, len(s))
	}
	j := strings.Index(s[start:], sub)
	if j < 0 {
		return -1
	}
	return start + j
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
