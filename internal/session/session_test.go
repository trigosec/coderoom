package session_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/session"
)

// mockAgent is a controllable agent.Agent for tests.
// Pre-load messages at construction; Stop() closes the read channel.
type mockAgent struct {
	once         sync.Once
	ch           chan agent.Message
	mu           sync.Mutex
	sends        []string
	startErr     error
	stopErr      error
	sendErr      error
	interruptErr error
	interrupts   int
}

func newMockAgent(msgs ...agent.Message) *mockAgent {
	m := &mockAgent{ch: make(chan agent.Message, max(len(msgs), 1))}
	for _, msg := range msgs {
		m.ch <- msg
	}
	return m
}

func (m *mockAgent) Start() error { return m.startErr }
func (m *mockAgent) Interrupt() error {
	m.mu.Lock()
	m.interrupts++
	m.mu.Unlock()
	return m.interruptErr
}
func (m *mockAgent) Stop() error {
	m.once.Do(func() { close(m.ch) })
	return m.stopErr
}
func (m *mockAgent) Send(text string) error {
	m.mu.Lock()
	m.sends = append(m.sends, text)
	m.mu.Unlock()
	return m.sendErr
}
func (m *mockAgent) SendNotice(text string) error {
	m.mu.Lock()
	m.sends = append(m.sends, text)
	m.mu.Unlock()
	return m.sendErr
}
func (m *mockAgent) Read() (agent.Message, error) {
	msg, ok := <-m.ch
	if !ok {
		return agent.Message{}, errors.New("agent closed")
	}
	return msg, nil
}

type gateAgent struct {
	startGate chan struct{}
	*mockAgent
}

func (g *gateAgent) Start() error {
	<-g.startGate
	return g.mockAgent.Start()
}

// testObserver collects events and exposes a channel for synchronisation.
type testObserver struct {
	mu     sync.Mutex
	events []session.Event
	ch     chan session.Event
}

func newTestObserver() *testObserver {
	return &testObserver{ch: make(chan session.Event, 128)}
}

func (o *testObserver) OnEvent(e session.Event) {
	o.mu.Lock()
	o.events = append(o.events, e)
	o.mu.Unlock()
	select {
	case o.ch <- e:
	default:
		// ch full; event still recorded in o.events — mustReceive will time out
		// rather than deadlock the reader goroutine.
	}
}

func mustReceive(t *testing.T, ch <-chan session.Event, want session.Kind) session.Event {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case ev := <-ch:
			if want == session.KindAgentStarted && ev.Kind == session.KindAgentStarting {
				continue
			}
			if want == session.KindAgentCrashed && ev.Kind == session.KindAgentLog {
				continue
			}
			if ev.Kind != want {
				t.Fatalf("expected kind %q, got %q", want, ev.Kind)
			}
			return ev
		case <-deadline:
			t.Fatalf("timed out waiting for %q event", want)
			return session.Event{}
		}
	}
}

// invite executes an InviteCommand for the given alias.
func invite(t *testing.T, s *session.Session, alias string) {
	t.Helper()
	err := s.Execute(session.InviteCommand{
		Alias:      alias,
		Role:       participant.RoleBuilder,
		Initiative: participant.InitiativeManual,
	})
	if err != nil {
		t.Fatalf("InviteCommand %q: %v", alias, err)
	}
}

// fixedFactory returns a session option whose factory always returns the given agent.
func fixedFactory(a agent.Agent) session.Option {
	return session.WithAgentFactory(func(_ string) agent.Agent { return a })
}

// mappedFactory returns a session option whose factory looks up agents by alias.
func mappedFactory(agents map[string]agent.Agent) session.Option {
	return session.WithAgentFactory(func(alias string) agent.Agent { return agents[alias] })
}

// --- tests ---

func TestInvite_emitsAgentStarted(t *testing.T) {
	obs := newTestObserver()
	a := newMockAgent()
	s := session.New(session.WithObserver(obs), fixedFactory(a))
	t.Cleanup(func() { _ = s.Execute(session.RemoveCommand{Alias: "ada"}) })

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarting)
	mustReceive(t, obs.ch, session.KindAgentStarted)
}

func TestCancel_interruptsAgent(t *testing.T) {
	obs := newTestObserver()
	a := newMockAgent()
	s := session.New(session.WithObserver(obs), fixedFactory(a))
	t.Cleanup(func() { _ = s.Execute(session.RemoveCommand{Alias: "ada"}) })

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarting)
	mustReceive(t, obs.ch, session.KindAgentStarted)

	if err := s.Execute(session.CancelCommand{Alias: "ada"}); err != nil {
		t.Fatalf("CancelCommand: %v", err)
	}
	a.mu.Lock()
	got := a.interrupts
	a.mu.Unlock()
	if got != 1 {
		t.Fatalf("expected 1 interrupt call, got %d", got)
	}
}

func TestCancel_notReadyWhileStarting(t *testing.T) {
	obs := newTestObserver()
	base := newMockAgent()
	g := &gateAgent{startGate: make(chan struct{}), mockAgent: base}
	s := session.New(session.WithObserver(obs), fixedFactory(g))
	t.Cleanup(func() { _ = s.Execute(session.RemoveCommand{Alias: "ada"}) })

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarting)

	if err := s.Execute(session.CancelCommand{Alias: "ada"}); err == nil {
		t.Fatalf("expected error cancelling starting agent")
	}

	close(g.startGate)
	mustReceive(t, obs.ch, session.KindAgentStarted)
}

func TestCancel_notReadyWhenCrashed(t *testing.T) {
	obs := newTestObserver()
	a := newMockAgent()
	a.startErr = errors.New("boom")
	s := session.New(session.WithObserver(obs), fixedFactory(a))

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarting)
	mustReceive(t, obs.ch, session.KindAgentCrashed)

	if err := s.Execute(session.CancelCommand{Alias: "ada"}); err == nil {
		t.Fatalf("expected error cancelling crashed agent")
	}
}

func TestCancel_unknownAlias(t *testing.T) {
	s := session.New()
	if err := s.Execute(session.CancelCommand{Alias: "nobody"}); err == nil {
		t.Fatalf("expected error for unknown participant")
	}
}

func TestInvite_colorStoredOnParticipant(t *testing.T) {
	obs := newTestObserver()
	a := newMockAgent()
	s := session.New(session.WithObserver(obs), fixedFactory(a))
	t.Cleanup(func() { _ = s.Execute(session.RemoveCommand{Alias: "ada"}) })

	err := s.Execute(session.InviteCommand{
		Alias:      "ada",
		Role:       participant.RoleBuilder,
		Initiative: participant.InitiativeManual,
		Color:      "#4ade80",
	})
	if err != nil {
		t.Fatalf("InviteCommand: %v", err)
	}
	mustReceive(t, obs.ch, session.KindAgentStarting)
	mustReceive(t, obs.ch, session.KindAgentStarted)
	p, ok := s.Participant("ada")
	if !ok {
		t.Fatal("participant not found after invite")
	}
	if p.Color != "#4ade80" {
		t.Errorf("expected color %q on participant, got %q", "#4ade80", p.Color)
	}
}

func TestInvite_duplicateAlias(t *testing.T) {
	s := session.New(session.WithAgentFactory(func(_ string) agent.Agent { return newMockAgent() }))
	t.Cleanup(func() { _ = s.Execute(session.RemoveCommand{Alias: "ada"}) })

	invite(t, s, "ada")
	err := s.Execute(session.InviteCommand{Alias: "ada", Role: participant.RoleBuilder, Initiative: participant.InitiativeManual})
	if err == nil {
		t.Fatal("expected error on duplicate alias, got nil")
	}
}

func TestRemove_emitsAgentStopped(t *testing.T) {
	obs := newTestObserver()
	a := newMockAgent()
	s := session.New(session.WithObserver(obs), fixedFactory(a))

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarting)
	mustReceive(t, obs.ch, session.KindAgentStarted)

	if err := s.Execute(session.RemoveCommand{Alias: "ada"}); err != nil {
		t.Fatalf("RemoveCommand: %v", err)
	}
	mustReceive(t, obs.ch, session.KindAgentStopped)
}

func TestRemove_notFound(t *testing.T) {
	s := session.New()
	if err := s.Execute(session.RemoveCommand{Alias: "nobody"}); err == nil {
		t.Fatal("expected error for unknown alias, got nil")
	}
}

func TestBroadcast_emitsAndSendsToAllAgents(t *testing.T) {
	obs := newTestObserver()
	a1 := newMockAgent()
	a2 := newMockAgent()
	s := session.New(session.WithObserver(obs), mappedFactory(map[string]agent.Agent{
		"ada":    a1,
		"turing": a2,
	}))
	t.Cleanup(func() {
		_ = s.Execute(session.RemoveCommand{Alias: "ada"})
		_ = s.Execute(session.RemoveCommand{Alias: "turing"})
	})

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarted)
	invite(t, s, "turing")
	mustReceive(t, obs.ch, session.KindAgentStarted)

	if err := s.Execute(session.BroadcastCommand{Text: "hello"}); err != nil {
		t.Fatalf("BroadcastCommand: %v", err)
	}
	mustReceive(t, obs.ch, session.KindBroadcast)

	for _, a := range []*mockAgent{a1, a2} {
		a.mu.Lock()
		if len(a.sends) == 0 || a.sends[len(a.sends)-1] != "hello" {
			t.Errorf("agent did not receive broadcast")
		}
		a.mu.Unlock()
	}

	for _, alias := range []string{"ada", "turing"} {
		p, ok := s.Participant(alias)
		if !ok {
			t.Fatalf("expected participant %q", alias)
		}
		if p.Status != participant.StatusWorking {
			t.Fatalf("expected %q status %q, got %q", alias, participant.StatusWorking, p.Status)
		}
	}
}

func TestBroadcast_sendError_doesNotMarkWorking(t *testing.T) {
	obs := newTestObserver()
	a1 := newMockAgent()
	a1.sendErr = errors.New("send failed")
	a2 := newMockAgent()
	s := session.New(session.WithObserver(obs), mappedFactory(map[string]agent.Agent{
		"ada":    a1,
		"turing": a2,
	}))
	t.Cleanup(func() {
		_ = s.Execute(session.RemoveCommand{Alias: "ada"})
		_ = s.Execute(session.RemoveCommand{Alias: "turing"})
	})

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarted)
	invite(t, s, "turing")
	mustReceive(t, obs.ch, session.KindAgentStarted)

	if err := s.Execute(session.BroadcastCommand{Text: "hello"}); err == nil {
		t.Fatal("expected broadcast error, got nil")
	}
	mustReceive(t, obs.ch, session.KindBroadcast)

	p, ok := s.Participant("ada")
	if !ok {
		t.Fatal("expected participant ada")
	}
	if p.Status == participant.StatusWorking {
		t.Fatalf("expected ada not to be marked %q on send error", participant.StatusWorking)
	}

	p, ok = s.Participant("turing")
	if !ok {
		t.Fatal("expected participant turing")
	}
	if p.Status != participant.StatusWorking {
		t.Fatalf("expected turing status %q, got %q", participant.StatusWorking, p.Status)
	}
}

func TestSharedSend_sendsToAddressedAndNotifiesOthers(t *testing.T) {
	obs := newTestObserver()
	ada := newMockAgent()
	turing := newMockAgent()
	s := session.New(session.WithObserver(obs), mappedFactory(map[string]agent.Agent{
		"ada":    ada,
		"turing": turing,
	}))
	t.Cleanup(func() {
		_ = s.Execute(session.RemoveCommand{Alias: "ada"})
		_ = s.Execute(session.RemoveCommand{Alias: "turing"})
	})

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarted)
	invite(t, s, "turing")
	mustReceive(t, obs.ch, session.KindAgentStarted)

	if err := s.Execute(session.SharedSendCommand{Alias: "ada", TextDirect: "do the thing", TextListeners: "ada is working on something"}); err != nil {
		t.Fatalf("SharedSendCommand: %v", err)
	}
	mustReceive(t, obs.ch, session.KindSharedSend)
	ev := mustReceive(t, obs.ch, session.KindSharedNotice)
	if ev.Alias != "turing" {
		t.Errorf("expected notice for turing, got %q", ev.Alias)
	}
	if ev.Text != "ada is working on something" {
		t.Errorf("unexpected notice text: %q", ev.Text)
	}

	ada.mu.Lock()
	if len(ada.sends) == 0 || ada.sends[len(ada.sends)-1] != "do the thing" {
		t.Errorf("addressed agent did not receive instruction")
	}
	ada.mu.Unlock()

	turing.mu.Lock()
	if len(turing.sends) == 0 || turing.sends[len(turing.sends)-1] != "ada is working on something" {
		t.Errorf("other agent did not receive context notice")
	}
	turing.mu.Unlock()
}

func TestSharedSend_sendError_doesNotMarkWorking(t *testing.T) {
	obs := newTestObserver()
	ada := newMockAgent()
	ada.sendErr = errors.New("send failed")
	turing := newMockAgent()
	s := session.New(session.WithObserver(obs), mappedFactory(map[string]agent.Agent{
		"ada":    ada,
		"turing": turing,
	}))
	t.Cleanup(func() {
		_ = s.Execute(session.RemoveCommand{Alias: "ada"})
		_ = s.Execute(session.RemoveCommand{Alias: "turing"})
	})

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarted)
	invite(t, s, "turing")
	mustReceive(t, obs.ch, session.KindAgentStarted)

	err := s.Execute(session.SharedSendCommand{Alias: "ada", TextDirect: "do the thing", TextListeners: "ada is working on something"})
	if err == nil {
		t.Fatal("expected shared send error, got nil")
	}

	p, ok := s.Participant("ada")
	if !ok {
		t.Fatal("expected participant ada")
	}
	if p.Status == participant.StatusWorking {
		t.Fatalf("expected ada not to be marked %q on send error", participant.StatusWorking)
	}

	p, ok = s.Participant("turing")
	if !ok {
		t.Fatal("expected participant turing")
	}
	if p.Status == participant.StatusWorking {
		t.Fatalf("expected turing not to be marked %q on send error", participant.StatusWorking)
	}
}

func TestSharedSend_notFound(t *testing.T) {
	s := session.New()
	if err := s.Execute(session.SharedSendCommand{Alias: "nobody", TextDirect: "hi", TextListeners: "hi"}); err == nil {
		t.Fatal("expected error for unknown alias, got nil")
	}
}

func TestPrivateSend_forwardsToAgentOnly(t *testing.T) {
	obs := newTestObserver()
	ada := newMockAgent()
	turing := newMockAgent()
	s := session.New(session.WithObserver(obs), mappedFactory(map[string]agent.Agent{
		"ada":    ada,
		"turing": turing,
	}))
	t.Cleanup(func() {
		_ = s.Execute(session.RemoveCommand{Alias: "ada"})
		_ = s.Execute(session.RemoveCommand{Alias: "turing"})
	})

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarted)
	invite(t, s, "turing")
	mustReceive(t, obs.ch, session.KindAgentStarted)

	if err := s.Execute(session.PrivateSendCommand{Alias: "ada", Text: "secret"}); err != nil {
		t.Fatalf("PrivateSendCommand: %v", err)
	}

	ada.mu.Lock()
	if len(ada.sends) == 0 || ada.sends[len(ada.sends)-1] != "secret" {
		t.Errorf("addressed agent did not receive message")
	}
	ada.mu.Unlock()

	turing.mu.Lock()
	if len(turing.sends) != 0 {
		t.Errorf("other agent should not receive private message")
	}
	turing.mu.Unlock()

	// no shared room event emitted
	select {
	case ev := <-obs.ch:
		t.Errorf("expected no shared room event, got %q", ev.Kind)
	default:
	}
}

func TestPrivateSend_notFound(t *testing.T) {
	s := session.New()
	if err := s.Execute(session.PrivateSendCommand{Alias: "nobody", Text: "hi"}); err == nil {
		t.Fatal("expected error for unknown alias, got nil")
	}
}

func TestReaderLoop_emitsDelta(t *testing.T) {
	obs := newTestObserver()
	a := newMockAgent(agent.Message{Kind: agent.MessageDelta, Text: "hello"})
	s := session.New(session.WithObserver(obs), fixedFactory(a))
	t.Cleanup(func() { _ = s.Execute(session.RemoveCommand{Alias: "ada"}) })

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarted)

	ev := mustReceive(t, obs.ch, session.KindDelta)
	if ev.Text != "hello" {
		t.Errorf("expected delta %q, got %q", "hello", ev.Text)
	}
}

func TestReaderLoop_emitsDone(t *testing.T) {
	obs := newTestObserver()
	a := newMockAgent(agent.Message{Kind: agent.MessageDone})
	s := session.New(session.WithObserver(obs), fixedFactory(a))
	t.Cleanup(func() { _ = s.Execute(session.RemoveCommand{Alias: "ada"}) })

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarted)
	mustReceive(t, obs.ch, session.KindDone)
}

func TestMultipleObservers_bothNotified(t *testing.T) {
	obs1 := newTestObserver()
	obs2 := newTestObserver()
	a := newMockAgent()
	s := session.New(session.WithObserver(obs1), session.WithObserver(obs2), fixedFactory(a))
	t.Cleanup(func() { _ = s.Execute(session.RemoveCommand{Alias: "ada"}) })

	invite(t, s, "ada")
	mustReceive(t, obs1.ch, session.KindAgentStarted)
	mustReceive(t, obs2.ch, session.KindAgentStarted)
}

func TestReaderLoop_emitsAgentLog(t *testing.T) {
	obs := newTestObserver()
	a := newMockAgent(agent.Message{Kind: agent.MessageLog, Text: "npm warn something"})
	s := session.New(session.WithObserver(obs), fixedFactory(a))
	t.Cleanup(func() { _ = s.Execute(session.RemoveCommand{Alias: "ada"}) })

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarted)

	ev := mustReceive(t, obs.ch, session.KindAgentLog)
	if ev.Text != "npm warn something" {
		t.Errorf("expected log text %q, got %q", "npm warn something", ev.Text)
	}
	if ev.Alias != "ada" {
		t.Errorf("expected alias %q, got %q", "ada", ev.Alias)
	}
}

func TestReaderLoop_agentCrash_emitsCrashed(t *testing.T) {
	obs := newTestObserver()
	a := newMockAgent() // no messages; Stop() will close channel
	_ = a.Stop()        // close immediately — simulates crash
	s := session.New(session.WithObserver(obs), fixedFactory(a))

	invite(t, s, "ada")
	mustReceive(t, obs.ch, session.KindAgentStarted)
	mustReceive(t, obs.ch, session.KindAgentCrashed)
}
