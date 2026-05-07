// Package session implements the Session Controller: the central orchestrator
// for command dispatch, message routing, and participant lifecycle.
// See docs/design/pkg-session.md for the full design rationale.
package session

import (
	"context"
	"fmt"
	"sync"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
)

type agentEntry struct {
	cancel context.CancelFunc
}

// Session is the central orchestrator of a Code Room session.
// Execute must be called from a single goroutine (the TUI input loop).
// It is not safe for concurrent calls to Execute.
type Session struct {
	mu       sync.Mutex
	registry *participant.Registry
	agents   map[string]agentEntry
	obs      []Observer
}

// Option configures a Session at construction time.
type Option func(*Session)

// WithObserver appends an Observer that receives all session events.
// May be called multiple times to register multiple observers.
func WithObserver(obs Observer) Option {
	return func(s *Session) { s.obs = append(s.obs, obs) }
}

// New returns an empty Session.
func New(opts ...Option) *Session {
	s := &Session{
		registry: participant.NewRegistry(),
		agents:   make(map[string]agentEntry),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Execute dispatches a command. Must be called from a single goroutine.
func (s *Session) Execute(cmd Command) error {
	return cmd.execute(s)
}

func (s *Session) notify(e Event) {
	for _, o := range s.obs {
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

func (s *Session) removeParticipant(alias string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.registry.Remove(alias)
}

// detachParticipant removes the participant and its reader entry atomically,
// cancels the reader context (signals readLoop: stopped, not crashed),
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
	entry.cancel()
	return p, true
}

func (s *Session) startReader(alias string, a agent.Agent) {
	ctx, cancel := context.WithCancel(context.Background()) //nolint:gosec // cancel stored in agentEntry; called inside detachParticipant
	s.mu.Lock()
	s.agents[alias] = agentEntry{cancel: cancel}
	s.mu.Unlock()
	go s.readLoop(ctx, alias, a)
}

func (s *Session) lookupParticipant(alias string) (*participant.Participant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.registry.Get(alias)
}

func (s *Session) participants() []*participant.Participant {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.registry.List()
}

// readLoop runs in a goroutine per agent, forwarding agent.Event values to the
// observers. When Read returns an error it emits KindAgentStopped (if shutdown
// was requested via ctx) or KindAgentCrashed, then exits.
func (s *Session) readLoop(ctx context.Context, alias string, a agent.Agent) {
	for {
		ev, err := a.Read()
		if err != nil {
			kind := KindAgentCrashed
			if ctx.Err() != nil {
				kind = KindAgentStopped
			}
			s.notify(Event{Kind: kind, Alias: alias})
			return
		}
		if ev.Delta != "" {
			s.notify(Event{Kind: KindDelta, Alias: alias, Text: ev.Delta})
		}
		if ev.Done {
			s.notify(Event{Kind: KindDone, Alias: alias})
		}
	}
}
