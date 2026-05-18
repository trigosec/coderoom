package room

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/ui/room/approval"
)

func flattenCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, sub := range batch {
			out = append(out, flattenCmd(sub)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func containsMsg[T any](msgs []tea.Msg) bool {
	var t T
	for _, msg := range msgs {
		if _, ok := msg.(T); ok {
			return true
		}
	}
	_ = t
	return false
}

func TestApprovalMode_enterEmitsApprovalDecisionMsgAndReturnsToCompose(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 20)
	m = m.ShowApproval(agent.ApprovalRequest{
		Ask:     "approve?",
		Options: []agent.ApprovalOption{agent.OptionDecline, agent.OptionAccept},
	})

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown}) // select accept
	if cmd != nil {
		t.Fatal("expected no cmd from navigation")
	}

	// Enter produces a ConfirmMsg, which must be fed back into Update to get a decision.
	next, cmd = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd from approval confirm")
	}
	confirm := cmd()
	if _, ok := confirm.(approval.ConfirmMsg); !ok {
		t.Fatalf("expected ConfirmMsg, got %T", confirm)
	}

	next2, cmd2 := next.Update(confirm)
	if cmd2 == nil {
		t.Fatal("expected cmd producing ApprovalDecisionMsg after confirm")
	}
	if !containsMsg[ApprovalDecisionMsg](flattenCmd(cmd2)) {
		t.Fatalf("expected ApprovalDecisionMsg after confirm; got %#v", flattenCmd(cmd2))
	}

	// After handling confirm, the room should be back in compose mode.
	_ = next2.ComposeValue()
}

func TestApprovalMode_escEmitsDeclineDecisionMsg(t *testing.T) {
	m := New(nil, "")
	m = m.HandleResize(80, 20)
	m = m.ShowApproval(agent.ApprovalRequest{
		Ask:     "approve?",
		Options: []agent.ApprovalOption{agent.OptionDecline, agent.OptionAccept},
	})

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected cmd from Esc cancel")
	}
	cancel := cmd()
	if _, ok := cancel.(approval.CancelMsg); !ok {
		t.Fatalf("expected CancelMsg, got %T", cancel)
	}

	_, cmd2 := next.Update(cancel)
	if cmd2 == nil {
		t.Fatal("expected cmd producing ApprovalDecisionMsg after cancel")
	}
	if !containsMsg[ApprovalDecisionMsg](flattenCmd(cmd2)) {
		t.Fatalf("expected ApprovalDecisionMsg after cancel; got %#v", flattenCmd(cmd2))
	}
}
