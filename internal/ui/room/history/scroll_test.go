package history

import (
	"testing"

	"github.com/trigosec/coderoom/internal/agent"
)

func TestDelta_whenAtBottom_keepsViewportAtBottom(t *testing.T) {
	m := New(nil, "")
	m = m.SetSize(80, 10)
	for range 25 {
		m = m.AppendSystemRecord("[x]")
	}
	m = m.GotoBottom()
	if !m.AtBottom() {
		t.Fatal("expected viewport at bottom before delta")
	}

	m = m.HandleAgentMessage("ada", agent.Message{StreamID: "s1", Mode: agent.ModeStream, Content: agent.Output{Text: "hello"}})
	if !m.AtBottom() {
		t.Fatal("expected delta to keep viewport at bottom when already at bottom")
	}
}

func TestDelta_whenScrolledUp_doesNotForceViewportToBottom(t *testing.T) {
	m := New(nil, "")
	m = m.SetSize(80, 10)
	for range 25 {
		m = m.AppendSystemRecord("[x]")
	}
	m = m.GotoBottom()
	m = m.ScrollUp(3)
	if m.AtBottom() {
		t.Fatal("expected viewport not at bottom after scrolling up")
	}
	y := m.YOffset()

	m = m.HandleAgentMessage("ada", agent.Message{StreamID: "s1", Mode: agent.ModeStream, Content: agent.Output{Text: "hello"}})
	if m.YOffset() != y {
		t.Fatalf("expected delta not to force viewport to bottom when scrolled up; yOffset changed from %d to %d", y, m.YOffset())
	}
}
