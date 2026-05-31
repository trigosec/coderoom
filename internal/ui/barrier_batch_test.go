package ui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/session"
	"github.com/trigosec/coderoom/internal/ui/room"
	"github.com/trigosec/coderoom/internal/ui/room/history/record"
)

type testAgent struct {
	ch        chan agent.Message
	sendErr   error
	sendCalls int
}

func newTestAgent() *testAgent {
	return &testAgent{ch: make(chan agent.Message, 16)}
}

func (a *testAgent) Start() error { return nil }

const testTurnAnchor = agent.StreamID("test:turn-anchor")

func (a *testAgent) Send(string) (agent.StreamID, error) {
	a.sendCalls++
	if a.sendErr != nil {
		return "", a.sendErr
	}
	return testTurnAnchor, nil
}
func (a *testAgent) SendNotice(string) (agent.StreamID, error) {
	// In this test stub, treat notices as a complete turn that ends immediately.
	const noticeTurnStream = agent.StreamID("codex:notice-turn")
	a.push(agent.Message{StreamID: noticeTurnStream, Mode: agent.ModeFlush, Content: agent.Output{}})
	return noticeTurnStream, nil
}
func (a *testAgent) Interrupt() error { return nil }
func (a *testAgent) Stop() error {
	close(a.ch)
	return nil
}
func (a *testAgent) Read() (agent.Message, error) {
	msg, ok := <-a.ch
	if !ok {
		return agent.Message{}, errors.New("agent closed")
	}
	return msg, nil
}

func (a *testAgent) push(msg agent.Message) { a.ch <- msg }

func mustPullEvent(t *testing.T, m *Model, timeout time.Duration) session.Event {
	t.Helper()
	ch := make(chan session.Event, 1)
	go func() {
		ev, ok := m.queue.Pull()
		if !ok {
			return
		}
		ch <- ev
	}()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(timeout):
		t.Fatal("timed out waiting for session event")
		return session.Event{}
	}
}

func pumpUntil(t *testing.T, m Model, pred func(session.Event) bool) Model {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ev := mustPullEvent(t, &m, 2*time.Second)
		next, _ := m.Update(sessionEventMsg(ev))
		m = next.(Model)
		if pred(ev) {
			return m
		}
	}
	t.Fatal("timed out pumping events")
	return m
}

func inviteParticipant(t *testing.T, s *session.Session, alias, color string) {
	t.Helper()
	if err := s.Execute(session.InviteCommand{
		Alias:      alias,
		Role:       participant.RoleBuilder,
		Initiative: participant.InitiativeManual,
		Color:      color,
	}); err != nil {
		t.Fatalf("invite %s: %v", alias, err)
	}
}

func pumpUntilAgentsStarted(t *testing.T, m Model, want ...string) Model {
	t.Helper()
	started := map[string]bool{}
	wantSet := map[string]bool{}
	for _, alias := range want {
		wantSet[alias] = true
	}
	return pumpUntil(t, m, func(ev session.Event) bool {
		if ev.Kind == session.KindAgentStarted {
			started[ev.Alias] = true
		}
		for alias := range wantSet {
			if !started[alias] {
				return false
			}
		}
		return true
	})
}

func assertHistoryDoesNotContainUserInput(t *testing.T, m Model, text string) {
	t.Helper()
	for _, r := range m.room.HistoryRecords() {
		if r.Kind == record.KindUserInput && r.Text == text {
			t.Fatalf("did not expect user input %q to be committed before dispatch", text)
		}
	}
}

func assertHistoryContainsUserInput(t *testing.T, m Model, text string) {
	t.Helper()
	for _, r := range m.room.HistoryRecords() {
		if r.Kind == record.KindUserInput && r.Text == text {
			return
		}
	}
	t.Fatalf("expected committed user input record after dispatch; records: %v", m.room.HistoryRecords())
}

func isIdleStatusChange(alias string) func(session.Event) bool {
	return func(ev session.Event) bool {
		return ev.Kind == session.KindParticipantStatusChanged &&
			ev.Alias == alias &&
			ev.StatusTo == participant.StatusIdle
	}
}

func isStreamOutput(alias, text string) func(session.Event) bool {
	return func(ev session.Event) bool {
		if ev.Kind != session.KindAgentMessage || ev.Alias != alias || ev.Msg == nil {
			return false
		}
		out, ok := ev.Msg.Content.(agent.Output)
		return ok && ev.Msg.Mode == agent.ModeStream && out.Text == text
	}
}

func TestBarrierBatch_stagesThenDispatchesWhenIdle(t *testing.T) {
	agents := map[string]*testAgent{
		"ada":    newTestAgent(),
		"turing": newTestAgent(),
	}
	s := session.New(session.WithAgentFactory(func(alias string) agent.Agent { return agents[alias] }))
	m := New(s, ".")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	// Invite two participants and pump until both are started.
	inviteParticipant(t, s, "ada", "#4ade80")
	inviteParticipant(t, s, "turing", "#60a5fa")
	m = pumpUntilAgentsStarted(t, m, "ada", "turing")

	// Mark ada working, leaving turing idle.
	if err := s.Execute(session.SharedSendCommand{Alias: "ada", TextDirect: "busy", TextListeners: "notice"}); err != nil {
		t.Fatalf("make ada busy: %v", err)
	}
	p, _ := s.Participant("ada")
	if p.Status != participant.StatusWorking {
		t.Fatalf("expected ada to be working, got %q", p.Status)
	}

	// Submit a broadcast; should stage, not dispatch.
	next, _ = m.Update(room.SubmitMsg{Text: "next turn"})
	m = next.(Model)
	if !m.room.HasStagedBatch() || !m.room.IsComposerStaged() {
		t.Fatalf("expected staged batch and staged composer")
	}
	assertHistoryDoesNotContainUserInput(t, m, "next turn")

	// Signal turn completion for ada and pump events until it becomes idle.
	agents["ada"].push(agent.Message{StreamID: "out1", Mode: agent.ModeStream, Content: agent.Output{Text: "done"}})
	agents["ada"].push(agent.Message{StreamID: "out1", Mode: agent.ModeFlush, Content: agent.Output{}})
	agents["ada"].push(agent.Message{StreamID: testTurnAnchor, Mode: agent.ModeFlush, Content: agent.Output{}})
	m = pumpUntil(t, m, isIdleStatusChange("ada"))

	if m.room.HasStagedBatch() || m.room.IsComposerStaged() {
		t.Fatalf("expected staged batch cleared after dispatch")
	}
	assertHistoryContainsUserInput(t, m, "next turn")
}

func TestBarrierBatch_autoDispatchPreservesFirstOutputRecord(t *testing.T) {
	agents := map[string]*testAgent{
		"ada":    newTestAgent(),
		"turing": newTestAgent(),
	}
	s := session.New(session.WithAgentFactory(func(alias string) agent.Agent { return agents[alias] }))
	m := New(s, ".")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	inviteParticipant(t, s, "ada", "#4ade80")
	inviteParticipant(t, s, "turing", "#60a5fa")
	m = pumpUntilAgentsStarted(t, m, "ada", "turing")

	if err := s.Execute(session.SharedSendCommand{Alias: "ada", TextDirect: "busy", TextListeners: "notice"}); err != nil {
		t.Fatalf("make ada busy: %v", err)
	}
	next, _ = m.Update(room.SubmitMsg{Text: "next turn"})
	m = next.(Model)
	if !m.room.HasStagedBatch() {
		t.Fatal("expected staged batch before ada becomes idle")
	}

	agents["ada"].push(agent.Message{
		StreamID: "turn-old-output",
		Mode:     agent.ModeStream,
		Content:  agent.Output{Text: "old output"},
	})
	agents["ada"].push(agent.Message{StreamID: "turn-old-output", Mode: agent.ModeFlush, Content: agent.Output{}})
	agents["ada"].push(agent.Message{StreamID: testTurnAnchor, Mode: agent.ModeFlush, Content: agent.Output{}})
	m = pumpUntil(t, m, isIdleStatusChange("ada"))

	agents["ada"].push(agent.Message{
		StreamID: "turn-new-output",
		Mode:     agent.ModeStream,
		Content:  agent.Output{Text: "fresh output"},
	})
	m = pumpUntil(t, m, isStreamOutput("ada", "fresh output"))

	if !hasRecord(m, record.KindAgentOutput, "fresh output") {
		t.Fatalf("expected first output fragment from auto-dispatched turn to appear in history; records: %v", m.room.HistoryRecords())
	}
}

func TestBarrierBatch_failedDispatchDoesNotCommitUserInput(t *testing.T) {
	agents := map[string]*testAgent{
		"ada": newTestAgent(),
	}
	agents["ada"].sendErr = errors.New("send failed")

	s := session.New(session.WithAgentFactory(func(alias string) agent.Agent { return agents[alias] }))
	m := New(s, ".")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	inviteParticipant(t, s, "ada", "#4ade80")
	m = pumpUntilAgentsStarted(t, m, "ada")

	next, _ = m.Update(room.SubmitMsg{Text: "next turn"})
	m = next.(Model)

	assertHistoryDoesNotContainUserInput(t, m, "next turn")
	if !hasRecord(m, record.KindSystem, `error: broadcast: broadcast to "ada"`) {
		t.Fatalf("expected dispatch error to be surfaced; records: %v", m.room.HistoryRecords())
	}
	if m.room.ComposeValue() != "next turn" {
		t.Fatalf("expected failed dispatch to preserve composer text, got %q", m.room.ComposeValue())
	}
	if m.room.HasStagedBatch() || m.room.IsComposerStaged() {
		t.Fatal("expected failed dispatch to clear staged state")
	}
}

func TestBarrierBatch_failedDispatchDoesNotRetryOnRollbackIdle(t *testing.T) {
	agents := map[string]*testAgent{
		"ada": newTestAgent(),
	}
	agents["ada"].sendErr = errors.New("send failed")

	s := session.New(session.WithAgentFactory(func(alias string) agent.Agent { return agents[alias] }))
	m := New(s, ".")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	inviteParticipant(t, s, "ada", "#4ade80")
	m = pumpUntilAgentsStarted(t, m, "ada")

	next, _ = m.Update(room.SubmitMsg{Text: "next turn"})
	m = next.(Model)
	if agents["ada"].sendCalls != 1 {
		t.Fatalf("expected initial dispatch attempt count 1, got %d", agents["ada"].sendCalls)
	}

	for range 3 {
		ev := mustPullEvent(t, &m, 2*time.Second)
		next, _ = m.Update(sessionEventMsg(ev))
		m = next.(Model)
	}

	if agents["ada"].sendCalls != 1 {
		t.Fatalf("expected no retry after rollback idle events, got %d send attempts", agents["ada"].sendCalls)
	}
	if m.room.HasStagedBatch() || m.room.IsComposerStaged() {
		t.Fatal("expected staged state to remain cleared after rollback events")
	}
}

func TestBarrierBatch_partialDispatchCommitsUserInput(t *testing.T) {
	agents := map[string]*testAgent{
		"ada":    newTestAgent(),
		"turing": newTestAgent(),
	}
	agents["ada"].sendErr = errors.New("send failed")

	s := session.New(session.WithAgentFactory(func(alias string) agent.Agent { return agents[alias] }))
	m := New(s, ".")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	inviteParticipant(t, s, "ada", "#4ade80")
	inviteParticipant(t, s, "turing", "#60a5fa")
	m = pumpUntilAgentsStarted(t, m, "ada", "turing")

	next, _ = m.Update(room.SubmitMsg{Text: "next turn"})
	m = next.(Model)

	assertHistoryContainsUserInput(t, m, "next turn")
	if !hasRecord(m, record.KindSystem, `error: broadcast: broadcast to "ada"`) {
		t.Fatalf("expected dispatch error to be surfaced; records: %v", m.room.HistoryRecords())
	}
	if m.room.ComposeValue() != "" {
		t.Fatalf("expected partial dispatch to clear composer, got %q", m.room.ComposeValue())
	}
	if m.room.HasStagedBatch() {
		t.Fatal("expected staged batch cleared after partial dispatch")
	}
}

func TestBarrierBatch_discardedTargetRestoresDraft(t *testing.T) {
	agents := map[string]*testAgent{
		"ada": newTestAgent(),
	}

	s := session.New(session.WithAgentFactory(func(alias string) agent.Agent { return agents[alias] }))
	m := New(s, ".")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	inviteParticipant(t, s, "ada", "#4ade80")
	m = pumpUntilAgentsStarted(t, m, "ada")

	if err := s.Execute(session.PrivateSendCommand{Alias: "ada", Text: "busy"}); err != nil {
		t.Fatalf("make ada busy: %v", err)
	}
	next, _ = m.Update(room.SubmitMsg{Text: "@ada hi"})
	m = next.(Model)
	if !m.room.HasStagedBatch() {
		t.Fatal("expected staged batch before target disappears")
	}

	if err := s.Execute(session.RemoveCommand{Alias: "ada"}); err != nil {
		t.Fatalf("remove ada: %v", err)
	}
	m = pumpUntil(t, m, func(ev session.Event) bool {
		return ev.Kind == session.KindAgentStopped && ev.Alias == "ada"
	})

	assertHistoryDoesNotContainUserInput(t, m, "@ada hi")
	if !hasRecord(m, record.KindSystem, "staged message discarded: no active targets") {
		t.Fatalf("expected staged discard message; records: %v", m.room.HistoryRecords())
	}
	if m.room.ComposeValue() != "@ada hi" {
		t.Fatalf("expected discarded staged send to restore draft, got %q", m.room.ComposeValue())
	}
	if m.room.HasStagedBatch() || m.room.IsComposerStaged() {
		t.Fatal("expected staged state cleared after target discard")
	}
}
