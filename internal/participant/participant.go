// Package participant defines the session-level entity: a named collaborator
// with a role, initiative level, and capabilities, backed by an agent process.
package participant

import (
	"time"

	"github.com/trigosec/coderoom/internal/agent"
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
	StatusIdle     Status = "idle"
	StatusStarting Status = "starting"
	StatusWorking  Status = "working"
	StatusCrashed  Status = "crashed"
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
}

// MarkStarting updates the participant status to StatusStarting and resets Since.
func (p *Participant) MarkStarting(now time.Time) {
	p.Status = StatusStarting
	p.Since = now
}

// MarkIdle updates the participant status to StatusIdle and resets Since.
func (p *Participant) MarkIdle(now time.Time) {
	p.Status = StatusIdle
	p.Since = now
}

// MarkWorking updates the participant status to StatusWorking and resets Since.
func (p *Participant) MarkWorking(now time.Time) {
	p.Status = StatusWorking
	p.Since = now
}

// MarkCrashed updates the participant status to StatusCrashed and resets Since.
func (p *Participant) MarkCrashed(now time.Time) {
	p.Status = StatusCrashed
	p.Since = now
}
