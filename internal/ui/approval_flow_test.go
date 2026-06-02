package ui

import (
	"strings"
	"testing"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/session"
	"github.com/trigosec/coderoom/internal/ui/room/staging"
)

func TestApprovalEvents_ClearActivePrompt(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{
		Kind:       session.KindApprovalRequested,
		Alias:      "ada",
		ApprovalID: 7,
		ApprovalReq: &agent.ApprovalRequest{
			Ask:     "approve?",
			Options: []agent.ApprovalOption{agent.OptionAccept, agent.OptionDecline},
		},
	})

	if !strings.Contains(m.room.View(), "approve?") {
		t.Fatalf("expected approval prompt in room view, got:\n%s", m.room.View())
	}

	m = pushEvent(m, session.Event{Kind: session.KindApprovalCleared, ApprovalID: 7})

	if strings.Contains(m.room.View(), "approve?") {
		t.Fatalf("expected cleared approval prompt to disappear, got:\n%s", m.room.View())
	}
	if m.activeApprovalID != 0 {
		t.Fatalf("active approval id = %d, want 0", m.activeApprovalID)
	}
}

func TestApprovalEvents_IgnoreClearForDifferentApproval(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{
		Kind:       session.KindApprovalRequested,
		Alias:      "ada",
		ApprovalID: 7,
		ApprovalReq: &agent.ApprovalRequest{
			Ask:     "approve?",
			Options: []agent.ApprovalOption{agent.OptionAccept, agent.OptionDecline},
		},
	})

	m = pushEvent(m, session.Event{Kind: session.KindApprovalCleared, ApprovalID: 8})

	if !strings.Contains(m.room.View(), "approve?") {
		t.Fatalf("expected mismatched clear event to preserve the visible prompt, got:\n%s", m.room.View())
	}
	if m.activeApprovalID != 7 {
		t.Fatalf("active approval id = %d, want 7", m.activeApprovalID)
	}
}

func TestApprovalEvents_ClearRestoresStagedComposer(t *testing.T) {
	m := makeReadyModel(t)
	batch := staging.NewBatch(
		"next turn",
		staging.Action{Kind: staging.ActionBroadcast, Text: "next turn"},
		[]string{"ada"},
	)
	m.room = m.room.StageBatch(batch, []string{"ada"})

	if !m.room.IsComposerStaged() || !m.room.HasStagedBatch() {
		t.Fatal("expected staged composer before approval prompt")
	}

	m = pushEvent(m, session.Event{
		Kind:       session.KindApprovalRequested,
		Alias:      "ada",
		ApprovalID: 7,
		ApprovalReq: &agent.ApprovalRequest{
			Ask:     "approve?",
			Options: []agent.ApprovalOption{agent.OptionAccept, agent.OptionDecline},
		},
	})
	m = pushEvent(m, session.Event{Kind: session.KindApprovalCleared, ApprovalID: 7})

	if !m.room.IsComposerStaged() {
		t.Fatal("expected staged composer to be restored after approval clear")
	}
	if !m.room.HasStagedBatch() {
		t.Fatal("expected staged batch to remain after approval clear")
	}
	if got := m.room.StagedBatch(); got != batch {
		t.Fatalf("staged batch pointer changed: got %#v want %#v", got, batch)
	}
	if strings.Contains(m.room.View(), "approve?") {
		t.Fatalf("expected cleared approval prompt to disappear, got:\n%s", m.room.View())
	}
}
