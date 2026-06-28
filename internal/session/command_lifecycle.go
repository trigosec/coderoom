package session

import (
	"errors"
	"fmt"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
)

// InviteCommand adds an agent to the session and starts it.
// The session uses its AgentFactory to construct the agent for the given alias.
type InviteCommand struct {
	Alias      string
	Role       participant.Role
	Initiative participant.Initiative
	Color      string // display colour assigned by the caller; stored on the participant
}

func (c InviteCommand) execute(s *Session) error {
	if s.agentFactory == nil {
		return fmt.Errorf("no agent factory configured on session")
	}
	a, p := c.buildInvite(s)
	if err := s.addParticipant(p); err != nil {
		return err
	}
	s.notify(ParticipantStatusChanged{Alias: c.Alias, From: "", To: p.Status, Since: p.Since})
	s.notify(AgentStarting{Alias: c.Alias})
	startInvitedAgent(c.Alias, a, s)
	return nil
}

func (c InviteCommand) buildInvite(s *Session) (agent.Agent, *participant.Participant) {
	a := s.agentFactory(s, c.Alias)
	p := &participant.Participant{
		Alias:      c.Alias,
		Role:       c.Role,
		Initiative: c.Initiative,
		Color:      c.Color,
	}
	p.BeginStartup(s.now())
	return a, p
}

func startInvitedAgent(alias string, a agent.Agent, s *Session) {
	go func() {
		if err := a.Start(); err != nil {
			handleInviteStartError(alias, err, s)
			return
		}
		attachStartedAgent(alias, a, s)
		s.startReader(alias, a)
		s.notify(AgentStarted{Alias: alias})
	}()
}

func handleInviteStartError(alias string, err error, s *Session) {
	s.notify(AgentLog{Alias: alias, Text: fmt.Sprintf("start failed: %v", err)})
	stateErr := s.updateParticipant(alias, func(p *participant.Participant) (Event, error) {
		from := p.Status
		p.Crash(s.now())
		return ParticipantStatusChanged{Alias: alias, From: from, To: p.Status, Since: p.Since}, nil
	})
	if stateErr != nil && !errors.Is(stateErr, errParticipantNotFound) {
		s.notifyParticipantInvariant(alias, stateErr)
	}
	s.notify(AgentCrashed{Alias: alias})
}

func attachStartedAgent(alias string, a agent.Agent, s *Session) {
	err := s.updateParticipant(alias, func(p *participant.Participant) (Event, error) {
		p.Agent = a
		from := p.Status
		if err := p.CompleteStartup(s.now()); err != nil {
			return nil, fmt.Errorf("complete startup: %w", err)
		}
		return ParticipantStatusChanged{Alias: alias, From: from, To: p.Status, Since: p.Since}, nil
	})
	if err != nil && !errors.Is(err, errParticipantNotFound) {
		s.notifyParticipantInvariant(alias, err)
	}
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
