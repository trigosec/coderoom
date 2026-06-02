// Package session implements the Session Controller: the central orchestrator
// for command dispatch, message routing, and participant lifecycle.
// See docs/design/pkg-session.md for the full design rationale.
package session

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
)

// AgentFactory constructs an agent.Agent for a given alias. The factory is
// responsible for wiring any backend-specific options (context, approval
// listener, logging) before returning.
//
// The session is passed to allow factories to use session-owned facilities
// (e.g., approval routing) without requiring UI-owned glue code.
type AgentFactory func(s *Session, alias string) agent.Agent

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
	approvals    *approvalHub
}

var errParticipantNotFound = errors.New("participant not found")

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
	s.approvals = newApprovalHub(s.notify)
	for _, o := range opts {
		o(s)
	}
	return s
}

// ApprovalListener returns an ApprovalListener that publishes approval requests
// as session events and blocks until the session resolves them.
func (s *Session) ApprovalListener(alias string) agent.ApprovalListener {
	return s.approvals.Listener(alias)
}

// Roster returns a snapshot of participants for UI display.
func (s *Session) Roster() []participant.Participant {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps := s.registry.List()
	out := make([]participant.Participant, len(ps))
	for i, p := range ps {
		out[i] = p.Snapshot()
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

func (s *Session) notifyParticipantInvariant(alias string, err error, details ...string) {
	if err == nil {
		return
	}
	text := "participant invariant: " + err.Error()
	if len(details) > 0 {
		cp := append([]string(nil), details...)
		filtered := cp[:0]
		for i := range cp {
			item := strings.TrimSpace(cp[i])
			if item == "" {
				continue
			}
			filtered = append(filtered, item)
		}
		if len(filtered) > 0 {
			text += " (" + strings.Join(filtered, "; ") + ")"
		}
	}
	s.notify(Event{Kind: KindAgentLog, Alias: alias, Text: text})
}

func summarizeAgentMessage(msg agent.Message) string {
	var payload string
	switch c := msg.Content.(type) {
	case agent.Output:
		payload = "output=" + truncateForLog(c.Text)
	case agent.Reasoning:
		payload = "reasoning=" + truncateForLog(c.Text)
	case agent.Command:
		if c.Command != "" {
			payload = "command=" + truncateForLog(c.Command)
		} else if c.Output != "" {
			payload = "command_output=" + truncateForLog(c.Output)
		}
	case agent.FileChangeSet:
		payload = fmt.Sprintf("file_changes=%d status=%q", len(c.Changes), c.Status)
	default:
		// agent.Log is handled earlier; keep the rest type-oriented.
		payload = fmt.Sprintf("content=%T", msg.Content)
	}
	if payload == "" {
		payload = fmt.Sprintf("content=%T", msg.Content)
	}
	return fmt.Sprintf("mode=%s stream=%q %s", agentModeString(msg.Mode), msg.StreamID, payload)
}

func agentModeString(m agent.Mode) string {
	switch m {
	case agent.ModeStream:
		return "stream"
	case agent.ModeFlush:
		return "flush"
	case agent.ModeSingle:
		return "single"
	default:
		return fmt.Sprintf("unknown(%d)", m)
	}
}

func truncateForLog(s string) string {
	const maxLen = 200
	if s == "" {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
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

func (s *Session) updateParticipant(alias string, fn func(*participant.Participant) (*Event, error)) error {
	var ev *Event
	var err error
	s.mu.Lock()
	p, ok := s.registry.Get(alias)
	if ok {
		ev, err = fn(p)
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: %q", errParticipantNotFound, alias)
	}
	if err != nil {
		return err
	}
	if ev != nil {
		s.notify(*ev)
	}
	return nil
}

// Participant returns a snapshot of the active participant with the given alias.
func (s *Session) Participant(alias string) (participant.Participant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.registry.Get(alias)
	if !ok {
		return participant.Participant{}, false
	}
	return p.Snapshot(), true
}

// Participants returns a snapshot of all currently active participants.
func (s *Session) Participants() []participant.Participant {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps := s.registry.List()
	out := make([]participant.Participant, len(ps))
	for i, p := range ps {
		out[i] = p.Snapshot()
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
		out[i] = p.Snapshot()
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
		err := s.updateParticipant(alias, func(p *participant.Participant) (*Event, error) {
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
		if err != nil && !errors.Is(err, errParticipantNotFound) {
			s.notifyParticipantInvariant(alias, err)
		}
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

// prepareParticipantForWork transitions the participant to Preparing state
// under the session lock. Must be followed by Send then beginParticipantWorking
// (on success) or abortWork (on Send failure).
func (s *Session) prepareParticipantForWork(alias string) error {
	return s.updateParticipant(alias, func(p *participant.Participant) (*Event, error) {
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
}

// beginParticipantWorking transitions from Preparing to Working and atomically
// tracks the turn-lifecycle anchor in OpenStreams. Call this after a successful
// Send to close the race window between PrepareForWork and anchor tracking.
func (s *Session) beginParticipantWorking(alias string, anchor agent.StreamID) {
	err := s.updateParticipant(alias, func(p *participant.Participant) (*Event, error) {
		from := p.Status
		if err := p.BeginWorking(s.now(), anchor); err != nil {
			return nil, fmt.Errorf("begin working: %w", err)
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

// abortWork rolls back a Preparing or Working participant to Idle. Use when
// Send fails after prepareParticipantForWork, or for error rollback paths.
func (s *Session) abortWork(alias string) {
	err := s.updateParticipant(alias, func(p *participant.Participant) (*Event, error) {
		if p.Status != participant.StatusWorking && p.Status != participant.StatusPreparing {
			return nil, nil
		}
		from := p.Status
		if err := p.AbortWork(s.now()); err != nil {
			return nil, fmt.Errorf("abort work: %w", err)
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

// markIdle transitions the participant to Idle via BecomeIdle. Called when the
// anchor stream flush is received (shouldIdle=true from CloseStream).
func (s *Session) markIdle(alias string) {
	err := s.updateParticipant(alias, func(p *participant.Participant) (*Event, error) {
		from := p.Status
		if err := p.BecomeIdle(s.now()); err != nil {
			return nil, fmt.Errorf("become idle: %w", err)
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

// trackAnchorStream tracks a stream for a participant that is already Working
// (e.g., a listener receiving a notice while an existing turn is in flight).
// Unlike beginParticipantWorking, this does not change the participant's status
// or replace its existing anchor.
func (s *Session) trackAnchorStream(alias string, streamID agent.StreamID) {
	if streamID == "" {
		return
	}
	s.mu.Lock()
	p, ok := s.registry.Get(alias)
	var err error
	if ok {
		err = p.TrackStream(streamID)
	}
	s.mu.Unlock()
	if err != nil {
		s.notifyParticipantInvariant(alias, err)
	}
}

func (s *Session) noteWorkingStreamMessage(alias string, msg agent.Message) (shouldIdle bool, sawTracked bool) {
	var err error
	s.mu.Lock()
	p, ok := s.registry.Get(alias)
	if !ok {
		s.mu.Unlock()
		return false, false
	}
	switch msg.Mode {
	case agent.ModeStream:
		err = p.TrackStream(msg.StreamID)
		s.mu.Unlock()
		if err != nil {
			s.notifyParticipantInvariant(alias, err, summarizeAgentMessage(msg))
			return false, false
		}
		return false, true
	case agent.ModeFlush:
		shouldIdle, err = p.CloseStream(msg.StreamID)
		s.mu.Unlock()
		if err != nil {
			// ErrStreamNotTracked on a flush is expected when belt-and-suspenders
			// close signals overlap (e.g. item/completed and turn/completed both
			// carry a close for the same stream). Silence it; other errors are
			// genuine invariant violations.
			if !errors.Is(err, participant.ErrStreamNotTracked) {
				s.notifyParticipantInvariant(alias, err, summarizeAgentMessage(msg))
			}
			return false, false
		}
		return shouldIdle, true
	default:
		s.mu.Unlock()
		return false, false
	}
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
	// Stream tracking only — no status transitions here. Transitions happen via
	// prepareParticipantForWork / beginParticipantWorking / markIdle driven by
	// the session command layer and the anchor stream close.
	switch msg.Content.(type) {
	case agent.Output, agent.Reasoning, agent.Command, agent.FileChangeSet:
		switch msg.Mode {
		case agent.ModeStream:
			_, _ = s.noteWorkingStreamMessage(alias, msg)
		case agent.ModeFlush:
			if shouldIdle, tracked := s.noteWorkingStreamMessage(alias, msg); tracked && shouldIdle {
				s.markIdle(alias)
			}
		case agent.ModeSingle:
			// Standalone messages are not stream-tracked.
		}
	}
}

func (s *Session) shouldDropIdleStreamFragment(alias string, msg agent.Message) bool {
	// Protocol guard:
	// Streaming fragments must belong to an active turn. If an adapter emits one
	// while the participant is idle, it is a lifecycle violation. Drop it to
	// avoid spuriously flipping the participant back to working and confusing
	// barrier-based UI features.
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
			Text:  "protocol: received stream fragment while idle; dropping (try cancel to resync if the agent is stuck; " + summarizeAgentMessage(msg) + ")",
		})
		return true
	default:
		return false
	}
}
