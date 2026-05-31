// Package participant defines the session-level entity: a named collaborator
// with a role, initiative level, and capabilities, backed by an agent process.
package participant

import (
	"errors"
	"fmt"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
)

var (
	// ErrWorkAlreadyStarted is returned when a participant is asked to start a
	// new turn while one is already in progress or being prepared.
	ErrWorkAlreadyStarted = errors.New("participant already working")
	// ErrStreamsStillOpen is returned when a participant is asked to become idle
	// before all tracked streams for the current turn have been closed.
	ErrStreamsStillOpen = errors.New("participant has open streams")
	// ErrNotWorking is returned when an operation requires the participant to be working.
	ErrNotWorking = errors.New("participant is not working")
	// ErrNotPreparing is returned when an operation requires the participant to be in preparing state.
	ErrNotPreparing = errors.New("participant is not in preparing state")
	// ErrStreamNotTracked is returned when a stream ID is not found in the participant's open streams.
	ErrStreamNotTracked = errors.New("participant stream is not tracked")
	// ErrNotReadyForWork is returned when a participant cannot accept work due to its current state.
	ErrNotReadyForWork = errors.New("participant not ready for work")
	// ErrNotStarting is returned when an operation requires the participant to be in starting state.
	ErrNotStarting = errors.New("participant is not starting")
	// ErrNotActive is returned when an operation requires the participant to be in Working or Preparing state.
	ErrNotActive = errors.New("participant is not active")
)

// Role defines the behavioural contract of a participant.
type Role string

// Supported roles.
const (
	RoleBuilder          Role = "builder"
	RoleReviewer         Role = "reviewer"
	RoleTester           Role = "tester"
	RoleArchitect        Role = "architect"
	RoleSecurityReviewer Role = "security-reviewer"
)

// Initiative controls how autonomously a participant acts.
type Initiative string

// Supported initiative levels.
const (
	InitiativeManual     Initiative = "manual"
	InitiativeActive     Initiative = "active"
	InitiativeAutonomous Initiative = "autonomous"
)

// Status is the runtime state of a participant's agent process.
type Status string

// Supported status values.
const (
	StatusIdle      Status = "idle"
	StatusStarting  Status = "starting"
	StatusPreparing Status = "preparing" // committed to a Send; anchor being established
	StatusWorking   Status = "working"
	StatusCrashed   Status = "crashed"
)

// Participant is a named collaborator in a session.
type Participant struct {
	Alias      string
	Role       Role
	Initiative Initiative
	Status     Status
	Color      string // hex colour code, e.g. "#4ade80"; empty means default terminal colour
	Agent      agent.Agent
	Since      time.Time
	// OpenStreams tracks the participant's active turn streams. Session is the
	// sole mutator; snapshots copy the map so observers/UI can inspect it safely.
	OpenStreams map[agent.StreamID]struct{}
	// anchor is the stream whose close signals turn completion. Set by
	// BeginWorking; cleared on BecomeIdle or AbortWork.
	anchor agent.StreamID
}

// Snapshot returns a value copy safe for observers/UI to inspect.
func (p *Participant) Snapshot() Participant {
	cp := *p
	cp.OpenStreams = cloneOpenStreams(p.OpenStreams)
	return cp
}

// BeginStartup transitions the participant into startup state.
func (p *Participant) BeginStartup(now time.Time) {
	p.resetOpenStreams()
	p.anchor = ""
	p.Status = StatusStarting
	p.Since = now
}

// CompleteStartup transitions a just-started participant from Starting to Idle.
// This is the startup lifecycle transition; it is distinct from BecomeIdle,
// which is the turn-completion transition.
func (p *Participant) CompleteStartup(now time.Time) error {
	if p.Status != StatusStarting {
		return ErrNotStarting
	}
	p.Status = StatusIdle
	p.Since = now
	return nil
}

// BecomeIdle transitions the participant from Working to Idle after a turn ends.
// The anchor must already be closed (removed from OpenStreams by CloseStream)
// before this is called.
func (p *Participant) BecomeIdle(now time.Time) error {
	if p.Status != StatusWorking {
		return ErrNotWorking
	}
	if p.anchor != "" {
		if _, open := p.OpenStreams[p.anchor]; open {
			return ErrStreamsStillOpen
		}
	} else if len(p.OpenStreams) > 0 {
		return ErrStreamsStillOpen
	}
	p.resetOpenStreams()
	p.anchor = ""
	p.Status = StatusIdle
	p.Since = now
	return nil
}

// Crash transitions the participant into crashed state.
func (p *Participant) Crash(now time.Time) {
	p.resetOpenStreams()
	p.anchor = ""
	p.Status = StatusCrashed
	p.Since = now
}

// PrepareForWork transitions the participant from Idle into Preparing.
// Resets OpenStreams and clears the anchor. Blocks concurrent sends.
func (p *Participant) PrepareForWork(now time.Time) error {
	switch p.Status {
	case StatusStarting, StatusCrashed:
		return ErrNotReadyForWork
	case StatusWorking, StatusPreparing:
		return ErrWorkAlreadyStarted
	case StatusIdle:
		// Ready to prepare.
	}
	p.resetOpenStreams()
	p.anchor = ""
	p.Status = StatusPreparing
	p.Since = now
	return nil
}

// BeginWorking transitions the participant from Preparing into Working and
// records the turn-lifecycle anchor. The anchor stream is added to OpenStreams
// so it is tracked from the moment Working begins — before the adapter can
// emit any messages — closing the race between Send returning and the first
// agent message arriving.
func (p *Participant) BeginWorking(now time.Time, anchor agent.StreamID) error {
	if p.Status != StatusPreparing {
		return ErrNotPreparing
	}
	p.anchor = anchor
	if anchor != "" {
		if p.OpenStreams == nil {
			p.OpenStreams = make(map[agent.StreamID]struct{})
		}
		p.OpenStreams[anchor] = struct{}{}
	}
	p.Status = StatusWorking
	p.Since = now
	return nil
}

// AbortWork rolls back a Preparing or Working state to Idle. Used when Send
// fails after PrepareForWork, or on error rollback from Working state.
func (p *Participant) AbortWork(now time.Time) error {
	if p.Status != StatusWorking && p.Status != StatusPreparing {
		return ErrNotWorking
	}
	p.resetOpenStreams()
	p.anchor = ""
	p.Status = StatusIdle
	p.Since = now
	return nil
}

// resetOpenStreams clears all active stream tracking for the participant.
func (p *Participant) resetOpenStreams() {
	p.OpenStreams = make(map[agent.StreamID]struct{})
}

// TrackStream records an active stream for the participant.
// Accepted from both StatusPreparing and StatusWorking so that messages
// arriving during the race window (Preparing) are tracked without error.
func (p *Participant) TrackStream(streamID agent.StreamID) error {
	if p.Status != StatusWorking && p.Status != StatusPreparing {
		return ErrNotActive
	}
	if p.OpenStreams == nil {
		p.OpenStreams = make(map[agent.StreamID]struct{})
	}
	p.OpenStreams[streamID] = struct{}{}
	return nil
}

// CloseStream removes a tracked stream.
// shouldIdle is true when the closed stream is the anchor and the participant
// is Working — this is the signal to call BecomeIdle.
// CloseStream is accepted from StatusWorking and StatusPreparing; it never
// returns shouldIdle=true from Preparing so premature idle is impossible.
func (p *Participant) CloseStream(streamID agent.StreamID) (shouldIdle bool, err error) {
	if p.Status != StatusWorking && p.Status != StatusPreparing {
		return false, ErrNotWorking
	}
	if _, ok := p.OpenStreams[streamID]; !ok {
		return false, fmt.Errorf("%w: %s", ErrStreamNotTracked, streamID)
	}
	delete(p.OpenStreams, streamID)
	if p.Status != StatusWorking {
		return false, nil
	}
	return streamID == p.anchor, nil
}

func cloneOpenStreams(in map[agent.StreamID]struct{}) map[agent.StreamID]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[agent.StreamID]struct{}, len(in))
	for streamID := range in {
		out[streamID] = struct{}{}
	}
	return out
}
