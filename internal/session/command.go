package session

import (
	"errors"
	"fmt"
	"slices"
	"strings"

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

// ResolveApprovalCommand resolves the active approval request and advances the
// session-managed approval queue.
type ResolveApprovalCommand struct {
	ApprovalID int64
	Choice     agent.ApprovalOption
}

func (c ResolveApprovalCommand) execute(s *Session) error {
	if s.approvals == nil {
		return fmt.Errorf("no approval hub configured on session")
	}
	if c.ApprovalID == 0 {
		return fmt.Errorf("approval id is required")
	}
	if !s.approvals.resolve(c.ApprovalID, c.Choice) {
		return fmt.Errorf("approval %d not active (already resolved, canceled, or queued)", c.ApprovalID)
	}
	return nil
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
		s.notify(Event{Kind: KindAgentStarted, Alias: alias})
	}()
}

func handleInviteStartError(alias string, err error, s *Session) {
	s.notify(Event{Kind: KindAgentLog, Alias: alias, Text: fmt.Sprintf("start failed: %v", err)})
	stateErr := s.updateParticipant(alias, func(p *participant.Participant) (*Event, error) {
		from := p.Status
		p.Crash(s.now())
		return &Event{
			Kind:       KindParticipantStatusChanged,
			Alias:      alias,
			StatusFrom: from,
			StatusTo:   p.Status,
			Since:      p.Since,
		}, nil
	})
	if stateErr != nil && !errors.Is(stateErr, errParticipantNotFound) {
		s.notifyParticipantInvariant(alias, stateErr)
	}
	s.notify(Event{Kind: KindAgentCrashed, Alias: alias})
}

func attachStartedAgent(alias string, a agent.Agent, s *Session) {
	err := s.updateParticipant(alias, func(p *participant.Participant) (*Event, error) {
		p.Agent = a
		from := p.Status
		if err := p.CompleteStartup(s.now()); err != nil {
			return nil, fmt.Errorf("complete startup: %w", err)
		}
		return &Event{
			Kind:       KindParticipantStatusChanged,
			Alias:      alias,
			StatusFrom: from,
			StatusTo:   p.Status,
			Since:      p.Since,
		}, nil
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

// BroadcastCommand sends a message to all agents.
type BroadcastCommand struct {
	Text string
}

func (c BroadcastCommand) execute(s *Session) error {
	s.notify(Event{Kind: KindBroadcast, Text: c.Text})
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
// controller does not construct or format messages. One KindSharedSend event
// is emitted to observers.
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
	s.notify(Event{Kind: KindSharedSend, Alias: c.Alias, Text: c.TextDirect})
	if err := sendSharedNotices(c.Alias, c.TextListeners, s); err != nil {
		return newDeliveryError([]string{c.Alias}, err)
	}
	return nil
}

// HandoffCommand transfers the latest completed room-visible output from one
// alias to another through a context path and emits a shared-room audit event.
type HandoffCommand struct {
	FromAlias     string
	ToAlias       string
	IdleAliases   []string
	ResolveSource func(alias string) (HandoffSource, bool)
}

func (c HandoffCommand) execute(s *Session) error {
	attempt := newHandoffAttempt(c, s)
	if err := c.validate(attempt, s); err != nil {
		return err
	}
	if err := c.resolveSource(attempt, s); err != nil {
		return err
	}
	return c.deliver(attempt, s)
}

type handoffAttempt struct {
	fromAlias string
	toAlias   string
	barrier   []string
	idle      []string
	busy      []string
	source    HandoffSource
}

func newHandoffAttempt(c HandoffCommand, s *Session) *handoffAttempt {
	barrier, idle, busy := handoffBarrierState(c.IdleAliases, s)
	return &handoffAttempt{
		fromAlias: c.FromAlias,
		toAlias:   c.ToAlias,
		barrier:   barrier,
		idle:      idle,
		busy:      busy,
		source:    HandoffSource{RecordIndex: -1},
	}
}

func (c HandoffCommand) validate(attempt *handoffAttempt, s *Session) error {
	if strings.TrimSpace(c.FromAlias) == "" || strings.TrimSpace(c.ToAlias) == "" {
		return rejectHandoffAttempt(attempt, s, "usage: /handoff <from> <to>", fmt.Errorf("usage: /handoff <from> <to>"))
	}
	if c.FromAlias == c.ToAlias {
		return rejectHandoffAttempt(attempt, s, "distinct aliases required", fmt.Errorf("handoff requires distinct source and destination aliases"))
	}
	if len(attempt.busy) > 0 {
		return rejectHandoffAttempt(
			attempt,
			s,
			"participants busy",
			fmt.Errorf("handoff requires all participants to be idle: %s", strings.Join(attempt.busy, ", ")),
		)
	}
	if c.ResolveSource == nil {
		return rejectHandoffAttempt(attempt, s, "source resolver missing", fmt.Errorf("handoff source resolver is required"))
	}
	return nil
}

func (c HandoffCommand) resolveSource(attempt *handoffAttempt, s *Session) error {
	source, ok := c.ResolveSource(c.FromAlias)
	if ok {
		attempt.source = source
		return nil
	}
	return rejectHandoffAttempt(
		attempt,
		s,
		"no completed room-visible output",
		fmt.Errorf("handoff source %q has no completed room-visible output", c.FromAlias),
	)
}

func (c HandoffCommand) deliver(attempt *handoffAttempt, s *Session) error {
	a, prepared, err := acquireParticipantForNotice(c.ToAlias, s)
	if err != nil {
		return rejectHandoffAttempt(attempt, s, err.Error(), err)
	}
	payload := formatHandoffPayload(c.FromAlias, attempt.source.Text)
	anchorID, err := a.SendNotice(payload)
	if err != nil {
		s.abortWork(c.ToAlias)
		return rejectHandoffAttempt(attempt, s, err.Error(), fmt.Errorf("handoff to %q: %w", c.ToAlias, err))
	}
	if prepared {
		s.beginParticipantWorking(c.ToAlias, anchorID)
	} else {
		s.trackAnchorStream(c.ToAlias, anchorID)
	}
	notifyHandoffDelivered(c.FromAlias, c.ToAlias, attempt, s)
	return nil
}

func rejectHandoffAttempt(attempt *handoffAttempt, s *Session, reason string, err error) error {
	notifyHandoffRejected(s, attempt.fromAlias, attempt.toAlias, attempt.barrier, attempt.idle, attempt.busy, attempt.source.RecordIndex, reason)
	return err
}

func notifyHandoffDelivered(fromAlias, toAlias string, attempt *handoffAttempt, s *Session) {
	s.notify(Event{
		Kind:              KindContextHandoff,
		FromAlias:         fromAlias,
		ToAlias:           toAlias,
		Text:              attempt.source.Text,
		Preview:           formatHandoffPreview(fromAlias, toAlias, attempt.source.Text),
		SourceRecordIndex: attempt.source.RecordIndex,
		BarrierAliases:    append([]string(nil), attempt.barrier...),
		IdleAliases:       append([]string(nil), attempt.idle...),
		BusyAliases:       append([]string(nil), attempt.busy...),
	})
	notifyHandoffAccepted(s, fromAlias, toAlias, attempt.barrier, attempt.idle, attempt.source.RecordIndex)
}

// acquireParticipantForDirectSend captures the participant's agent and
// transitions it from Idle to Preparing. Direct sends require exclusivity:
// an already-working participant rejects the command.
func acquireParticipantForDirectSend(alias string, s *Session) (a agent.Agent, err error) {
	err = s.updateParticipant(alias, func(p *participant.Participant) (*Event, error) {
		if p.Agent == nil || p.Status == participant.StatusStarting || p.Status == participant.StatusCrashed {
			return nil, nil
		}
		a = p.Agent
		from := p.Status
		if err := p.PrepareForWork(s.now()); err != nil {
			return nil, fmt.Errorf("prepare for work: %w", err)
		}
		return &Event{
			Kind:       KindParticipantStatusChanged,
			Alias:      alias,
			StatusFrom: from,
			StatusTo:   p.Status,
			Since:      p.Since,
		}, nil
	})
	if err != nil {
		if errors.Is(err, errParticipantNotFound) {
			return nil, fmt.Errorf("participant %q not found", alias)
		}
		s.notifyParticipantInvariant(alias, err)
		return nil, fmt.Errorf("participant %q invalid working transition: %w", alias, err)
	}
	if a == nil {
		return nil, fmt.Errorf("participant %q not ready", alias)
	}
	return a, nil
}

func handoffBarrierState(aliases []string, s *Session) (barrier []string, idle []string, busy []string) {
	if len(aliases) == 0 {
		for _, p := range s.BarrierParticipants() {
			barrier = append(barrier, p.Alias)
		}
	} else {
		barrier = append([]string(nil), aliases...)
		slices.Sort(barrier)
	}
	for _, alias := range barrier {
		p, ok := s.Participant(alias)
		if !ok {
			continue
		}
		if p.Status == participant.StatusIdle {
			idle = append(idle, alias)
			continue
		}
		busy = append(busy, alias)
	}
	return barrier, idle, busy
}

func notifyHandoffAccepted(s *Session, fromAlias, toAlias string, barrier, idle []string, sourceRecordIndex int) {
	s.notify(Event{
		Kind:              KindAgentLog,
		Text:              formatHandoffAttemptLog("accepted", fromAlias, toAlias, barrier, idle, nil, sourceRecordIndex, ""),
		FromAlias:         fromAlias,
		ToAlias:           toAlias,
		SourceRecordIndex: sourceRecordIndex,
		BarrierAliases:    append([]string(nil), barrier...),
		IdleAliases:       append([]string(nil), idle...),
	})
}

func notifyHandoffRejected(s *Session, fromAlias, toAlias string, barrier, idle, busy []string, sourceRecordIndex int, reason string) {
	reason = compactHandoffReason(reason)
	s.notify(Event{
		Kind:              KindAgentLog,
		Text:              formatHandoffAttemptLog("rejected", fromAlias, toAlias, barrier, idle, busy, sourceRecordIndex, reason),
		FromAlias:         fromAlias,
		ToAlias:           toAlias,
		SourceRecordIndex: sourceRecordIndex,
		BarrierAliases:    append([]string(nil), barrier...),
		IdleAliases:       append([]string(nil), idle...),
		BusyAliases:       append([]string(nil), busy...),
		RejectionReason:   reason,
	})
}

func compactHandoffReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	lines := strings.FieldsFunc(reason, func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts = append(parts, line)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " | ")
}

func formatHandoffAttemptLog(status, fromAlias, toAlias string, barrier, idle, busy []string, sourceRecordIndex int, reason string) string {
	var sb strings.Builder
	sb.WriteString("handoff ")
	sb.WriteString(status)
	sb.WriteString(": from=")
	sb.WriteString(fromAlias)
	sb.WriteString(" to=")
	sb.WriteString(toAlias)
	sb.WriteString(" barrier=")
	sb.WriteString(formatHandoffAliasList(barrier))
	sb.WriteString(" idle=")
	sb.WriteString(formatHandoffAliasList(idle))
	if len(busy) > 0 {
		sb.WriteString(" busy=")
		sb.WriteString(formatHandoffAliasList(busy))
	}
	sb.WriteString(" source_record=")
	if sourceRecordIndex < 0 {
		sb.WriteString("none")
	} else {
		fmt.Fprintf(&sb, "%d", sourceRecordIndex)
	}
	if strings.TrimSpace(reason) != "" {
		sb.WriteString(" reason=")
		sb.WriteString(reason)
	}
	return sb.String()
}

func formatHandoffAliasList(aliases []string) string {
	if len(aliases) == 0 {
		return "[]"
	}
	return "[" + strings.Join(aliases, ",") + "]"
}

func formatHandoffPayload(fromAlias string, text string) string {
	return fmt.Sprintf("[HANDOFF from %s]\n\n%s", fromAlias, text)
}

func formatHandoffPreview(fromAlias, toAlias, text string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "[handoff %s -> %s]", fromAlias, toAlias)
	sb.WriteString("\n  ↦ source: ")
	sb.WriteString(fromAlias)
	sb.WriteString(" latest output")

	preview, remaining := handoffBodyPreview(text)
	if preview != "" {
		sb.WriteString("\n")
		sb.WriteString(preview)
	}
	if remaining > 0 {
		fmt.Fprintf(&sb, "\n  (+%d more lines; Ctrl+G open transcript)", remaining)
	}
	return sb.String()
}

const handoffPreviewLines = 3
const handoffPreviewMaxCols = 120

func handoffBodyPreview(text string) (string, int) {
	body := strings.TrimRight(text, "\n")
	if body == "" {
		return "", 0
	}
	lines := strings.Split(body, "\n")
	previewCount := min(len(lines), handoffPreviewLines)
	preview := make([]string, 0, previewCount)
	for i := 0; i < previewCount; i++ {
		preview = append(preview, "  > "+truncateHandoffPreviewLine(lines[i], handoffPreviewMaxCols))
	}
	return strings.Join(preview, "\n"), len(lines) - previewCount
}

func truncateHandoffPreviewLine(s string, maxCols int) string {
	runes := []rune(s)
	if maxCols <= 0 || len(runes) <= maxCols {
		return s
	}
	if maxCols == 1 {
		return "…"
	}
	return string(runes[:maxCols-1]) + "…"
}

// acquireParticipantForNotice captures the participant's agent and transitions
// it to Preparing only when currently Idle. Existing Working/Preparing turns are
// allowed so a notice can be layered onto an active turn.
func acquireParticipantForNotice(alias string, s *Session) (a agent.Agent, prepared bool, err error) {
	err = s.updateParticipant(alias, func(p *participant.Participant) (*Event, error) {
		if p.Agent == nil || p.Status == participant.StatusStarting || p.Status == participant.StatusCrashed {
			return nil, nil
		}
		a = p.Agent
		if p.Status == participant.StatusWorking || p.Status == participant.StatusPreparing {
			return nil, nil
		}
		from := p.Status
		if err := p.PrepareForWork(s.now()); err != nil {
			return nil, fmt.Errorf("prepare for work: %w", err)
		}
		prepared = true
		return &Event{
			Kind:       KindParticipantStatusChanged,
			Alias:      alias,
			StatusFrom: from,
			StatusTo:   p.Status,
			Since:      p.Since,
		}, nil
	})
	if err != nil {
		if errors.Is(err, errParticipantNotFound) {
			return nil, false, fmt.Errorf("participant %q not found", alias)
		}
		s.notifyParticipantInvariant(alias, err)
		return nil, false, fmt.Errorf("participant %q invalid working transition: %w", alias, err)
	}
	if a == nil {
		return nil, false, fmt.Errorf("participant %q not ready", alias)
	}
	return a, prepared, nil
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
