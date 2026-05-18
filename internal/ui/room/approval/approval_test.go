package approval

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/agent"
)

func TestUpdate_downSelectsNextOption(t *testing.T) {
	m := New().Set(agent.ApprovalRequest{
		Ask:     "approve?",
		Options: []agent.ApprovalOption{agent.OptionDecline, agent.OptionAccept},
	})
	if got := m.Selected(); got != 0 {
		t.Fatalf("expected selected=0, got %d", got)
	}
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := m2.Selected(); got != 1 {
		t.Fatalf("expected selected=1 after Down, got %d", got)
	}
}

func TestUpdate_enterEmitsConfirmMsg(t *testing.T) {
	m := New().Set(agent.ApprovalRequest{
		Ask:     "approve?",
		Options: []agent.ApprovalOption{agent.OptionDecline},
	})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected confirm cmd")
	}
	if _, ok := cmd().(ConfirmMsg); !ok {
		t.Fatalf("expected ConfirmMsg, got %T", cmd())
	}
}
