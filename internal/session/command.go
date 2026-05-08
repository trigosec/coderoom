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
		Status:     participant.StatusRunning,
		Color:      c.Color,
		Agent:      c.Agent,
	}
	if err := s.addParticipant(p); err != nil {
		return err
	}
	if err := c.Agent.Start(); err != nil {
		s.removeParticipant(c.Alias)
		return fmt.Errorf("start agent %q: %w", c.Alias, err)
	}
	s.startReader(c.Alias, c.Agent)
	s.notify(Event{Kind: KindAgentStarted, Alias: c.Alias})
	return nil
}

// StopCommand stops and removes an agent from the session.
type StopCommand struct {
	Alias string
}

func (c StopCommand) execute(s *Session) error {
	p, ok := s.detachParticipant(c.Alias)
	if !ok {
		return fmt.Errorf("participant %q not found", c.Alias)
	}
	if err := p.Agent.Stop(); err != nil {
		return fmt.Errorf("stop agent %q: %w", c.Alias, err)
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
	for _, p := range s.participants() {
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
	p, ok := s.lookupParticipant(c.Alias)
	if !ok {
		return fmt.Errorf("participant %q not found", c.Alias)
	}
	if err := p.Agent.Send(c.TextDirect); err != nil {
		return fmt.Errorf("send to %q: %w", c.Alias, err)
	}
	s.notify(Event{Kind: KindSharedSend, Alias: c.Alias, Text: c.TextDirect})
	var errs []error
	for _, other := range s.participants() {
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
