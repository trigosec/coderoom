// Package session implements the Session Controller: the central orchestrator
// for command dispatch, message routing, and participant lifecycle.
// See docs/design/pkg-session.md for the full design rationale.
package session

import (
	"fmt"
	"sync"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
)

// AgentFactory constructs an agent.Agent for a given alias. The factory is
// responsible for wiring any backend-specific options (context, approval
// listener, logging) before returning.
type AgentFactory func(alias string) agent.Agent

type agentEntry struct {
	stop chan struct{}
}

// Session is the central orchestrator of a Code Room session.
// Execute must be called from a single goroutine (the TUI input loop).
// It is not safe for concurrent calls to Execute.
type Session struct {
	mu           sync.Mutex
	registry     *participant.Registry
	agents       map[string]agentEntry
	obs          []Observer
	now          func() time.Time
	agentFactory AgentFactory
}

// Option configures a Session at construction time.
type Option func(*Session)

// WithObserver appends an Observer that receives all session events.
// May be called multiple times to register multiple observers.
func WithObserver(obs Observer) Option {
	return func(s *Session) { s.obs = append(s.obs, obs) }
}

// AddObserver appends an Observer after construction.
func (s *Session) AddObserver(obs Observer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.obs = append(s.obs, obs)
}

// WithAgentFactory sets the factory used to construct agents when a participant
// is invited. The factory receives the agent alias and is responsible for
// wiring all backend options (context, approval listener, etc.).
func WithAgentFactory(f AgentFactory) Option {
	return func(s *Session) { s.agentFactory = f }
}

// New returns an empty Session.
func New(opts ...Option) *Session {
	s := &Session{
		registry: participant.NewRegistry(),
		agents:   make(map[string]agentEntry),
		now:      time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Roster returns a snapshot of participants for UI display.
func (s *Session) Roster() []participant.Participant {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps := s.registry.List()
	out := make([]participant.Participant, len(ps))
	for i, p := range ps {
		out[i] = *p
	}
	return out
}

// Execute dispatches a command. Must be called from a single goroutine.
func (s *Session) Execute(cmd Command) error {
	return cmd.execute(s)
}

func (s *Session) snapshotAgentsToStop() []agent.Agent {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps := s.registry.List()
	out := make([]agent.Agent, 0, len(ps))
	for _, p := range ps {
		if p.Agent == nil {
			continue
		}
		out = append(out, p.Agent)
	}
	return out
}

// Shutdown stops all agents in the session. Errors from individual agents are
// silently discarded; the goal is best-effort cleanup on process exit.
func (s *Session) Shutdown() {
	// Note: participants in StatusStarting may not have Agent set yet (we only
	// assign it after a successful Start()). Those in-flight processes are not
	// currently stoppable via Session.Shutdown.
	for _, a := range s.snapshotAgentsToStop() {
		_ = a.Stop()
	}
}

func (s *Session) notify(e Event) {
	s.mu.Lock()
	obs := s.obs
	s.mu.Unlock()
	for _, o := range obs {
		o.OnEvent(e)
	}
}

func (s *Session) addParticipant(p *participant.Participant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.registry.Add(p); err != nil {
		return fmt.Errorf("register participant: %w", err)
	}
	return nil
}

// detachParticipant removes the participant and its reader entry atomically,
// closes the stop channel (signals readLoop: stopped, not crashed),
// and returns the participant so the caller can stop the agent.
func (s *Session) detachParticipant(alias string) (*participant.Participant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.agents[alias]
	if !ok {
		return nil, false
	}
	p, _ := s.registry.Get(alias)
	delete(s.agents, alias)
	_ = s.registry.Remove(alias)
	close(entry.stop)
	return p, true
}

func (s *Session) startReader(alias string, a agent.Agent) {
	stop := make(chan struct{})
	s.mu.Lock()
	s.agents[alias] = agentEntry{stop: stop}
	s.mu.Unlock()
	go s.readLoop(stop, alias, a)
}

func (s *Session) lookupParticipant(alias string) (*participant.Participant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.registry.Get(alias)
}

func (s *Session) updateParticipant(alias string, fn func(*participant.Participant) *Event) bool {
	var ev *Event
	s.mu.Lock()
	p, ok := s.registry.Get(alias)
	if ok {
		ev = fn(p)
	}
	s.mu.Unlock()
	if !ok {
		return false
	}
	if ev != nil {
		s.notify(*ev)
	}
	return true
}

// Participant returns a snapshot of the active participant with the given alias.
func (s *Session) Participant(alias string) (participant.Participant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.registry.Get(alias)
	if !ok {
		return participant.Participant{}, false
	}
	return *p, true
}

// Participants returns a snapshot of all currently active participants.
func (s *Session) Participants() []participant.Participant {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps := s.registry.List()
	out := make([]participant.Participant, len(ps))
	for i, p := range ps {
		out[i] = *p
	}
	return out
}

// HasAnyActivityParticipants reports whether any participant is in a status
// that requires the activity monitor to tick.
func (s *Session) HasAnyActivityParticipants() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.registry.HasStarting() || s.registry.HasWorking() || s.registry.HasCrashed()
}

// RoutableParticipants returns a snapshot of participants that are safe to send
// messages to (agent started and not crashed).
func (s *Session) RoutableParticipants() []participant.Participant {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps := s.registry.ListAvailable()
	out := make([]participant.Participant, len(ps))
	for i, p := range ps {
		out[i] = *p
	}
	return out
}

// readLoop runs in a goroutine per agent, forwarding agent.Message values to the
// observers. When Read returns an error it emits KindAgentStopped (if the stop
// channel was closed) or KindAgentCrashed, then exits.
func (s *Session) readLoop(stop <-chan struct{}, alias string, a agent.Agent) {
	for {
		msg, err := a.Read()
		if err != nil {
			s.handleAgentReadError(stop, alias)
			return
		}
		s.handleAgentMessage(alias, msg)
	}
}

func (s *Session) handleAgentReadError(stop <-chan struct{}, alias string) {
	kind := s.kindForReadError(stop)
	if kind == KindAgentCrashed {
		s.updateParticipant(alias, func(p *participant.Participant) *Event {
			from := p.Status
			p.MarkCrashed(s.now())
			return &Event{
				Kind:       KindParticipantStatusChanged,
				Alias:      alias,
				StatusFrom: from,
				StatusTo:   p.Status,
				Since:      p.Since,
			}
		})
	}
	s.notify(Event{Kind: kind, Alias: alias})
}

func (s *Session) kindForReadError(stop <-chan struct{}) Kind {
	select {
	case <-stop:
		return KindAgentStopped
	default:
		return KindAgentCrashed
	}
}

func (s *Session) markWorking(alias string) {
	s.updateParticipant(alias, func(p *participant.Participant) *Event {
		if p.Status == participant.StatusWorking {
			return nil
		}
		from := p.Status
		p.MarkWorking(s.now())
		return &Event{
			Kind:       KindParticipantStatusChanged,
			Alias:      alias,
			StatusFrom: from,
			StatusTo:   p.Status,
			Since:      p.Since,
		}
	})
}

func (s *Session) markIdleIfWorking(alias string) {
	s.updateParticipant(alias, func(p *participant.Participant) *Event {
		if p.Status != participant.StatusWorking {
			return nil
		}
		from := p.Status
		p.MarkIdle(s.now())
		return &Event{
			Kind:       KindParticipantStatusChanged,
			Alias:      alias,
			StatusFrom: from,
			StatusTo:   p.Status,
			Since:      p.Since,
		}
	})
}

func (s *Session) participantStatus(alias string) (participant.Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.registry.Get(alias)
	if !ok {
		return "", false
	}
	return p.Status, true
}

func (s *Session) handleAgentMessage(alias string, msg agent.Message) {
	if s.shouldDropIdleStreamFragment(alias, msg) {
		return
	}

	if c, ok := msg.Content.(agent.Log); ok {
		if c.Text != "" {
			s.notify(Event{Kind: KindAgentLog, Alias: alias, Text: c.Text})
		}
		return
	}

	s.applyTurnLifecycle(alias, msg)
	m := msg
	s.notify(Event{Kind: KindAgentMessage, Alias: alias, Msg: &m})
}

func (s *Session) applyTurnLifecycle(alias string, msg agent.Message) {
	switch msg.Content.(type) {
	case agent.Output:
		switch msg.Mode {
		case agent.ModeStream:
			s.markWorking(alias)
		case agent.ModeFlush:
			s.updateParticipant(alias, func(p *participant.Participant) *Event {
				from := p.Status
				p.MarkIdle(s.now())
				return &Event{
					Kind:       KindParticipantStatusChanged,
					Alias:      alias,
					StatusFrom: from,
					StatusTo:   p.Status,
					Since:      p.Since,
				}
			})
		default:
		}
	case agent.Reasoning:
		if msg.Mode == agent.ModeStream {
			s.markWorking(alias)
		}
	case agent.Command, agent.FileChangeSet:
		// Tool/item streams are turn-scoped output. Treat their stream fragments
		// as activity so participant status is conservative even if an adapter
		// emits only tool deltas (no Output/Reasoning fragments).
		if msg.Mode == agent.ModeStream {
			s.markWorking(alias)
		}
	default:
	}
}

func (s *Session) shouldDropIdleStreamFragment(alias string, msg agent.Message) bool {
	// Protocol guard:
	// This codebase treats Output+ModeFlush as "turn completed" (see agent.SendAndWait
	// docstring). If an adapter emits a streaming fragment while the participant is
	// idle, it is a turn-lifecycle violation. Drop it to avoid spuriously flipping
	// the participant back to working and confusing barrier-based UI features.
	//
	// Note: we allow agent.Log unconditionally (it is not turn-scoped).
	if msg.Mode != agent.ModeStream {
		return false
	}
	st, ok := s.participantStatus(alias)
	if !ok || st != participant.StatusIdle {
		return false
	}
	switch msg.Content.(type) {
	case agent.Output, agent.Reasoning, agent.Command, agent.FileChangeSet:
		s.notify(Event{
			Kind:  KindAgentLog,
			Alias: alias,
			Text:  "protocol: received stream fragment while idle; dropping (try cancel to resync if the agent is stuck)",
		})
		return true
	default:
		return false
	}
}
