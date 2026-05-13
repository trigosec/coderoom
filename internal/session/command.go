package session

import (
	"errors"
	"fmt"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
)

// Command is a sealed interface; only types in this package can implement it.
// Execute dispatches via the unexported method — no type switch required.
type Command interface {
	execute(s *Session) error
}

// InviteCommand adds an agent to the session and starts it.
// The caller constructs the agent.Agent before passing it in.
type InviteCommand struct {
	Alias      string
	Agent      agent.Agent
	Role       participant.Role
	Initiative participant.Initiative
	Color      string // display colour assigned by the caller; stored on the participant
}

func (c InviteCommand) execute(s *Session) error {
	p := &participant.Participant{
		Alias:      c.Alias,
		Role:       c.Role,
		Initiative: c.Initiative,
		Color:      c.Color,
	}
	p.MarkStarting(s.now())
	if err := s.addParticipant(p); err != nil {
		return err
	}
	s.notify(Event{Kind: KindAgentStarting, Alias: c.Alias})
	go func(alias string, a agent.Agent) {
		if err := a.Start(); err != nil {
			s.notify(Event{Kind: KindAgentLog, Alias: alias, Text: fmt.Sprintf("start failed: %v", err)})
			s.withParticipant(alias, func(p *participant.Participant) {
				p.MarkCrashed(s.now())
			})
			s.notify(Event{Kind: KindAgentCrashed, Alias: alias})
			return
		}
		s.withParticipant(alias, func(p *participant.Participant) {
			p.Agent = a
			p.MarkIdle(s.now())
		})
		s.startReader(alias, a)
		s.notify(Event{Kind: KindAgentStarted, Alias: alias})
	}(c.Alias, c.Agent)
	return nil
}

// RemoveCommand stops and removes an agent from the session.
type RemoveCommand struct {
	Alias string
}

func (c RemoveCommand) execute(s *Session) error {
	p, ok := s.detachParticipant(c.Alias)
	if !ok {
		return fmt.Errorf("participant %q not found", c.Alias)
	}
	if err := p.Agent.Stop(); err != nil {
		return fmt.Errorf("remove agent %q: %w", c.Alias, err)
	}
	return nil
}

// CancelCommand requests an agent to interrupt its current in-flight work.
// The agent remains in the session.
type CancelCommand struct {
	Alias string
}

func (c CancelCommand) execute(s *Session) error {
	p, ok := s.lookupParticipant(c.Alias)
	if !ok {
		return fmt.Errorf("participant %q not found", c.Alias)
	}
	if p.Agent == nil || p.Status == participant.StatusStarting || p.Status == participant.StatusCrashed {
		return fmt.Errorf("participant %q not ready", c.Alias)
	}
	if err := p.Agent.Interrupt(); err != nil {
		return fmt.Errorf("interrupt %q: %w", c.Alias, err)
	}
	return nil
}

// BroadcastCommand sends a message to the shared room and to all agents.
// All agents receive the broadcast; initiative governs whether they may
// take action without being explicitly addressed.
type BroadcastCommand struct {
	Text string
}

func (c BroadcastCommand) execute(s *Session) error {
	s.notify(Event{Kind: KindBroadcast, Text: c.Text})
	var errs []error
	for _, p := range s.RoutableParticipants() {
		if err := p.Agent.Send(c.Text); err != nil {
			errs = append(errs, fmt.Errorf("broadcast to %q: %w", p.Alias, err))
		}
	}
	return errors.Join(errs...)
}

// SharedSendCommand sends a message to one agent in the shared room.
// TextDirect is sent to the addressed agent; TextListeners is sent to all
// other agents. The caller is responsible for both texts — the session
// controller does not construct or format messages. One KindSharedSend event
// is emitted to observers.
type SharedSendCommand struct {
	Alias         string
	TextDirect    string
	TextListeners string
}

func (c SharedSendCommand) execute(s *Session) error {
	var a agent.Agent
	if ok := s.withParticipant(c.Alias, func(p *participant.Participant) {
		if p.Agent == nil || p.Status == participant.StatusStarting || p.Status == participant.StatusCrashed {
			return
		}
		a = p.Agent
		p.MarkWorking(s.now())
	}); !ok {
		return fmt.Errorf("participant %q not found", c.Alias)
	}
	if a == nil {
		return fmt.Errorf("participant %q not ready", c.Alias)
	}
	if err := a.Send(c.TextDirect); err != nil {
		return fmt.Errorf("send to %q: %w", c.Alias, err)
	}
	s.notify(Event{Kind: KindSharedSend, Alias: c.Alias, Text: c.TextDirect})
	var errs []error
	for _, other := range s.RoutableParticipants() {
		if other.Alias == c.Alias {
			continue
		}
		if err := other.Agent.Send(c.TextListeners); err != nil {
			errs = append(errs, fmt.Errorf("notice to %q: %w", other.Alias, err))
			continue
		}
		s.notify(Event{Kind: KindSharedNotice, Alias: other.Alias, Text: c.TextListeners})
	}
	return errors.Join(errs...)
}

// PrivateSendCommand sends a message directly to one agent's private channel.
// Nothing is emitted to the shared room and no other agents are notified.
// Used for approval flows and reasoning that should not pollute the shared room.
type PrivateSendCommand struct {
	Alias string
	Text  string
}

func (c PrivateSendCommand) execute(s *Session) error {
	p, ok := s.lookupParticipant(c.Alias)
	if !ok {
		return fmt.Errorf("participant %q not found", c.Alias)
	}
	if err := p.Agent.Send(c.Text); err != nil {
		return fmt.Errorf("send to %q: %w", c.Alias, err)
	}
	return nil
}
