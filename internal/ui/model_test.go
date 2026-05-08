package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/session"
)

// makeReadyModel returns a Model that has processed one WindowSizeMsg so the
// viewport is initialised and syncViewport calls are live.
func makeReadyModel(t *testing.T) Model {
	t.Helper()
	m := New(".")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(Model)
}

// pushEvent sends a session event into the model via Update and returns the result.
func pushEvent(m Model, e session.Event) Model {
	next, _ := m.Update(sessionEventMsg(e))
	return next.(Model)
}

// --- channelObserver ---

func TestChannelObserver_forwardsToQueue(t *testing.T) {
	q := newEventQueue()
	obs := channelObserver{queue: q}
	go obs.OnEvent(session.Event{Kind: session.KindAgentStarted, Alias: "ada"})
	got, ok := q.Pull()
	if !ok {
		t.Fatal("queue closed unexpectedly")
	}
	if got.Alias != "ada" {
		t.Errorf("expected alias ada, got %q", got.Alias)
	}
}

// --- handleEvent: roster and line rendering ---

func TestHandleEvent_agentStarted(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentStarted, Alias: "ada"})
	if !slices.Contains(m.lines, "[ada joined]") {
		t.Errorf("expected [ada joined] in lines: %v", m.lines)
	}
	if !slices.Contains(m.agents, "ada") {
		t.Errorf("expected ada in agents: %v", m.agents)
	}
}

func TestHandleEvent_agentStopped(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentStarted, Alias: "ada"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if !slices.Contains(m.lines, "[ada left]") {
		t.Errorf("expected [ada left] in lines: %v", m.lines)
	}
	if slices.Contains(m.agents, "ada") {
		t.Errorf("ada should have been removed from agents: %v", m.agents)
	}
}

func TestHandleEvent_agentCrashed(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentStarted, Alias: "ada"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentCrashed, Alias: "ada"})
	if !slices.Contains(m.lines, "[ada crashed]") {
		t.Errorf("expected [ada crashed] in lines: %v", m.lines)
	}
	if slices.Contains(m.agents, "ada") {
		t.Errorf("ada should have been removed from agents: %v", m.agents)
	}
}

func TestHandleEvent_lineRendering(t *testing.T) {
	tests := []struct {
		name  string
		event session.Event
		want  string
	}{
		{"broadcast", session.Event{Kind: session.KindBroadcast, Text: "hello"}, "[all] hello"},
		{"sharedSend", session.Event{Kind: session.KindSharedSend, Alias: "ada", Text: "do it"}, "[-> ada] do it"},
		{"sharedNotice", session.Event{Kind: session.KindSharedNotice, Alias: "ada"}, "[notice -> ada]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := makeReadyModel(t)
			m = pushEvent(m, tt.event)
			if !slices.Contains(m.lines, tt.want) {
				t.Errorf("expected %q in lines: %v", tt.want, m.lines)
			}
		})
	}
}

// --- streaming: delta in-place append and KindDone ---

func TestHandleDelta_firstDeltaCreatesLine(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	if len(m.lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(m.lines))
	}
	if m.lines[0] != "ada> hello" {
		t.Errorf("expected 'ada> hello', got %q", m.lines[0])
	}
	if _, ok := m.streaming["ada"]; !ok {
		t.Error("expected ada to be marked as streaming")
	}
}

func TestHandleDelta_subsequentDeltaAppendsInPlace(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: " world"})
	if len(m.lines) != 1 {
		t.Fatalf("expected 1 line (in-place append), got %d", len(m.lines))
	}
	if m.lines[0] != "ada> hello world" {
		t.Errorf("expected 'ada> hello world', got %q", m.lines[0])
	}
}

func TestHandleDelta_twoAgentsStreamConcurrently(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "a"})
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "bob", Text: "b"})
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "2"})
	if len(m.lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(m.lines), m.lines)
	}
	if m.lines[m.streaming["ada"]] != "ada> a2" {
		t.Errorf("ada line wrong: %q", m.lines[m.streaming["ada"]])
	}
	if m.lines[m.streaming["bob"]] != "bob> b" {
		t.Errorf("bob line wrong: %q", m.lines[m.streaming["bob"]])
	}
}

func TestHandleEvent_kindDoneClearsStreaming(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	m = pushEvent(m, session.Event{Kind: session.KindDone, Alias: "ada"})
	if _, ok := m.streaming["ada"]; ok {
		t.Error("expected streaming to be cleared after KindDone")
	}
}

func TestHandleEvent_agentStoppedClearsStreaming(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "mid-stream"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if _, ok := m.streaming["ada"]; ok {
		t.Error("streaming should be cleared when agent stops mid-turn")
	}
}

// --- showWho / showHelp ---

func TestShowWho_noAgents(t *testing.T) {
	m := makeReadyModel(t)
	m = m.showWho()
	if !slices.Contains(m.lines, "[no agents]") {
		t.Errorf("expected [no agents] in lines: %v", m.lines)
	}
}

func TestShowWho_listsActiveAgents(t *testing.T) {
	m := makeReadyModel(t)
	m.agents = []string{"ada", "bob"}
	m = m.showWho()
	if !slices.Contains(m.lines, "[agents] ada, bob") {
		t.Errorf("expected [agents] ada, bob in lines: %v", m.lines)
	}
}

func TestShowHelp_coversAllCommands(t *testing.T) {
	m := makeReadyModel(t)
	m = m.showHelp()
	for _, cmd := range []string{"/invite", "/stop", "/who", "/help", "@<alias>", "/quit"} {
		found := false
		for _, line := range m.lines {
			if strings.Contains(line, cmd) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("help output missing %q; lines: %v", cmd, m.lines)
		}
	}
}

// --- removeAlias ---

func TestRemoveAlias_removesFromRosterAndStreaming(t *testing.T) {
	m := makeReadyModel(t)
	m.agents = []string{"ada", "bob", "charlie"}
	m.streaming["ada"] = 0
	m = m.removeAlias("ada")
	if slices.Contains(m.agents, "ada") {
		t.Errorf("ada should be removed from agents: %v", m.agents)
	}
	if len(m.agents) != 2 {
		t.Errorf("expected 2 remaining agents, got %d: %v", len(m.agents), m.agents)
	}
	if _, ok := m.streaming["ada"]; ok {
		t.Error("ada should be removed from streaming map")
	}
}

func TestRemoveAlias_preservesOthers(t *testing.T) {
	m := makeReadyModel(t)
	m.agents = []string{"ada", "bob"}
	m = m.removeAlias("ada")
	if !slices.Contains(m.agents, "bob") {
		t.Errorf("bob should still be in agents: %v", m.agents)
	}
}
