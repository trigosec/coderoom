package session

import (
	"errors"
	"fmt"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
)

// BroadcastCommand sends a message to all agents.
type BroadcastCommand struct {
	Text string
}

func (c BroadcastCommand) execute(s *Session) error {
	s.notify(Broadcast(c))
	var errs []error
	var delivered []string
	for _, p := range s.RoutableParticipants() {
		err := s.prepareParticipantForWork(p.Alias)
		if err != nil {
			if !errors.Is(err, errParticipantNotFound) {
				s.notifyParticipantInvariant(p.Alias, err)
			}
			errs = append(errs, fmt.Errorf("broadcast to %q: %w", p.Alias, err))
			continue
		}
		anchorID, err := p.Agent.Send(c.Text)
		if err != nil {
			s.abortWork(p.Alias)
			errs = append(errs, fmt.Errorf("broadcast to %q: %w", p.Alias, err))
			continue
		}
		s.beginParticipantWorking(p.Alias, anchorID)
		delivered = append(delivered, p.Alias)
	}
	joined := errors.Join(errs...)
	if joined != nil {
		return newDeliveryError(delivered, joined)
	}
	return nil
}

// SharedSendCommand sends a message to one agent in the shared room.
// TextDirect is sent to the addressed agent; TextListeners is sent to all
// other agents. The caller is responsible for both texts — the session
// controller does not construct or format messages. One SharedSend event is
// emitted to observers.
type SharedSendCommand struct {
	Alias         string
	TextDirect    string
	TextListeners string
}

func (c SharedSendCommand) execute(s *Session) error {
	a, err := acquireParticipantForDirectSend(c.Alias, s)
	if err != nil {
		return err
	}
	if err := sendPreparedDirect(c.Alias, a, c.TextDirect, s); err != nil {
		return err
	}
	s.notify(SharedSend{Alias: c.Alias, Text: c.TextDirect})
	if err := sendSharedNotices(c.Alias, c.TextListeners, s); err != nil {
		return newDeliveryError([]string{c.Alias}, err)
	}
	return nil
}

// PrivateSendCommand sends a message directly to one agent's private channel.
// Nothing is emitted to the shared room and no other agents are notified.
// Used for approval flows and reasoning that should not pollute the shared room.
type PrivateSendCommand struct {
	Alias string
	Text  string
}

func (c PrivateSendCommand) execute(s *Session) error {
	a, err := acquireParticipantForDirectSend(c.Alias, s)
	if err != nil {
		return err
	}
	anchorID, sendErr := a.Send(c.Text)
	if sendErr != nil {
		s.abortWork(c.Alias)
		return fmt.Errorf("send to %q: %w", c.Alias, sendErr)
	}
	s.beginParticipantWorking(c.Alias, anchorID)
	return nil
}

// acquireParticipantForDirectSend captures the participant's agent and
// transitions it from Idle to Preparing. Direct sends require exclusivity:
// an already-working participant rejects the command.
func acquireParticipantForDirectSend(alias string, s *Session) (a agent.Agent, err error) {
	if err := s.prepareParticipantForWork(alias); err != nil {
		return nil, formatDirectSendPrepareError(alias, err, s)
	}
	p, ok := s.lookupParticipant(alias)
	if !ok || p.Agent == nil || !p.IsSendable() {
		return nil, fmt.Errorf("participant %q not ready", alias)
	}
	return p.Agent, nil
}

// acquireParticipantForNotice captures the participant's agent and transitions
// it to Preparing only when currently Idle. Existing Working/Preparing turns are
// allowed so a notice can be layered onto an active turn.
func acquireParticipantForNotice(alias string, s *Session) (a agent.Agent, prepared bool, err error) {
	p, err := lookupSendableParticipant(alias, s)
	if err != nil {
		return nil, false, err
	}
	if isActiveParticipantStatus(p.Status) {
		return p.Agent, false, nil
	}
	a, err = acquireParticipantForDirectSend(alias, s)
	if err != nil {
		return nil, false, err
	}
	return a, true, nil
}

func formatDirectSendPrepareError(alias string, err error, s *Session) error {
	if errors.Is(err, errParticipantNotFound) {
		return fmt.Errorf("participant %q not found", alias)
	}
	s.notifyParticipantInvariant(alias, err)
	return fmt.Errorf("participant %q invalid working transition: %w", alias, err)
}

func lookupSendableParticipant(alias string, s *Session) (participant.Participant, error) {
	p, ok := s.lookupParticipant(alias)
	if !ok {
		return participant.Participant{}, fmt.Errorf("participant %q not found", alias)
	}
	if !p.IsSendable() || p.Agent == nil {
		return participant.Participant{}, fmt.Errorf("participant %q not ready", alias)
	}
	return p.Snapshot(), nil
}

func isActiveParticipantStatus(status participant.Status) bool {
	return status == participant.StatusWorking || status == participant.StatusPreparing
}

func sendPreparedDirect(alias string, a agent.Agent, text string, s *Session) error {
	anchorID, err := a.Send(text)
	if err != nil {
		s.abortWork(alias)
		return fmt.Errorf("send to %q: %w", alias, err)
	}
	s.beginParticipantWorking(alias, anchorID)
	return nil
}

func sendSharedNotices(addressedAlias string, text string, s *Session) error {
	var errs []error
	for _, other := range s.RoutableParticipants() {
		if other.Alias == addressedAlias {
			continue
		}
		a, prepared, err := acquireParticipantForNotice(other.Alias, s)
		if err != nil {
			errs = append(errs, fmt.Errorf("notice to %q: %w", other.Alias, err))
			continue
		}
		anchorID, err := a.SendNotice(text)
		if err != nil {
			s.abortWork(other.Alias)
			errs = append(errs, fmt.Errorf("notice to %q: %w", other.Alias, err))
			continue
		}
		if prepared {
			s.beginParticipantWorking(other.Alias, anchorID)
		} else {
			s.trackAnchorStream(other.Alias, anchorID)
		}
		s.notify(SharedNotice{Alias: other.Alias, Text: text})
	}
	return errors.Join(errs...)
}
