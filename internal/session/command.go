package session

import (
	"errors"
	"fmt"
	"slices"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
)

// DeliveryError reports that a command partially succeeded before returning an
// error. Delivered aliases accepted the turn; Err describes the failed
// deliveries.
type DeliveryError struct {
	Delivered []string
	Err       error
}

func (e *DeliveryError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *DeliveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newDeliveryError(delivered []string, err error) error {
	if err == nil {
		return nil
	}
	cp := append([]string(nil), delivered...)
	slices.Sort(cp)
	return &DeliveryError{Delivered: cp, Err: err}
}

// DeliveredAliases returns the aliases that accepted a turn before err was
// returned. It returns nil when err does not carry partial-delivery metadata.
func DeliveredAliases(err error) []string {
	var deliveryErr *DeliveryError
	if !errors.As(err, &deliveryErr) || deliveryErr == nil {
		return nil
	}
	return append([]string(nil), deliveryErr.Delivered...)
}

// Command is a sealed interface; only types in this package can implement it.
// Execute dispatches via the unexported method — no type switch required.
type Command interface {
	execute(s *Session) error
}

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
	s.notify(Event{
		Kind:       KindParticipantStatusChanged,
		Alias:      c.Alias,
		StatusFrom: "",
		StatusTo:   p.Status,
		Since:      p.Since,
	})
	s.notify(Event{Kind: KindAgentStarting, Alias: c.Alias})
	startInvitedAgent(c.Alias, a, s)
	return nil
}

func (c InviteCommand) buildInvite(s *Session) (agent.Agent, *participant.Participant) {
	a := s.agentFactory(c.Alias)
	p := &participant.Participant{
		Alias:      c.Alias,
		Role:       c.Role,
		Initiative: c.Initiative,
		Color:      c.Color,
	}
	p.MarkStarting(s.now())
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
		s.notify(Event{Kind: KindAgentStarted, Alias: alias})
	}()
}

func handleInviteStartError(alias string, err error, s *Session) {
	s.notify(Event{Kind: KindAgentLog, Alias: alias, Text: fmt.Sprintf("start failed: %v", err)})
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
	s.notify(Event{Kind: KindAgentCrashed, Alias: alias})
}

func attachStartedAgent(alias string, a agent.Agent, s *Session) {
	s.updateParticipant(alias, func(p *participant.Participant) *Event {
		p.Agent = a
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

// BroadcastCommand sends a message to all agents.
type BroadcastCommand struct {
	Text string
}

func (c BroadcastCommand) execute(s *Session) error {
	s.notify(Event{Kind: KindBroadcast, Text: c.Text})
	var errs []error
	var delivered []string
	for _, p := range s.RoutableParticipants() {
		s.updateParticipant(p.Alias, func(p *participant.Participant) *Event {
			from := p.Status
			p.MarkWorking(s.now())
			return &Event{
				Kind:       KindParticipantStatusChanged,
				Alias:      p.Alias,
				StatusFrom: from,
				StatusTo:   p.Status,
				Since:      p.Since,
			}
		})
		if err := p.Agent.Send(c.Text); err != nil {
			s.markIdleIfWorking(p.Alias)
			errs = append(errs, fmt.Errorf("broadcast to %q: %w", p.Alias, err))
			continue
		}
		delivered = append(delivered, p.Alias)
	}
	return newDeliveryError(delivered, errors.Join(errs...))
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
	a, err := acquireParticipantAndMarkWorking(c.Alias, s)
	if err != nil {
		return err
	}
	if err := sendSharedDirect(c.Alias, a, c.TextDirect, s); err != nil {
		return err
	}
	s.notify(Event{Kind: KindSharedSend, Alias: c.Alias, Text: c.TextDirect})
	if err := sendSharedNotices(c.Alias, c.TextListeners, s); err != nil {
		return newDeliveryError([]string{c.Alias}, err)
	}
	return nil
}

func acquireParticipantAndMarkWorking(alias string, s *Session) (agent.Agent, error) {
	var a agent.Agent
	if ok := s.updateParticipant(alias, func(p *participant.Participant) *Event {
		if p.Agent == nil || p.Status == participant.StatusStarting || p.Status == participant.StatusCrashed {
			return nil
		}
		a = p.Agent
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
	}); !ok {
		return nil, fmt.Errorf("participant %q not found", alias)
	}
	if a == nil {
		return nil, fmt.Errorf("participant %q not ready", alias)
	}
	return a, nil
}

func sendSharedDirect(alias string, a agent.Agent, text string, s *Session) error {
	if err := a.Send(text); err != nil {
		s.markIdleIfWorking(alias)
		return fmt.Errorf("send to %q: %w", alias, err)
	}
	return nil
}

func sendSharedNotices(addressedAlias string, text string, s *Session) error {
	var errs []error
	for _, other := range s.RoutableParticipants() {
		if other.Alias == addressedAlias {
			continue
		}
		a, err := acquireParticipantAndMarkWorking(other.Alias, s)
		if err != nil {
			errs = append(errs, fmt.Errorf("notice to %q: %w", other.Alias, err))
			continue
		}
		if err := a.SendNotice(text); err != nil {
			s.markIdleIfWorking(other.Alias)
			errs = append(errs, fmt.Errorf("notice to %q: %w", other.Alias, err))
			continue
		}
		s.notify(Event{Kind: KindSharedNotice, Alias: other.Alias, Text: text})
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
	var a agent.Agent
	if ok := s.updateParticipant(c.Alias, func(p *participant.Participant) *Event {
		if p.Agent == nil || p.Status == participant.StatusStarting || p.Status == participant.StatusCrashed {
			return nil
		}
		a = p.Agent
		from := p.Status
		p.MarkWorking(s.now())
		return &Event{
			Kind:       KindParticipantStatusChanged,
			Alias:      c.Alias,
			StatusFrom: from,
			StatusTo:   p.Status,
			Since:      p.Since,
		}
	}); !ok {
		return fmt.Errorf("participant %q not found", c.Alias)
	}
	if a == nil {
		return fmt.Errorf("participant %q not ready", c.Alias)
	}
	if err := a.Send(c.Text); err != nil {
		s.markIdleIfWorking(c.Alias)
		return fmt.Errorf("send to %q: %w", c.Alias, err)
	}
	return nil
}
