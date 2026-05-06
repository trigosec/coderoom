// Package event defines the session-level events appended to events.jsonl.
package event

import "time"

// Type identifies the kind of event.
type Type string

// Session-level event types appended to events.jsonl.
const (
	TypeSessionCreated   Type = "SessionCreated"
	TypeAgentInvited     Type = "AgentInvited"
	TypeAgentStarted     Type = "AgentStarted"
	TypeMessageSent      Type = "MessageSent"
	TypeCommandIssued    Type = "CommandIssued"
	TypeFileChanged      Type = "FileChanged"
	TypeAgentCrashed     Type = "AgentCrashed"
	TypeAgentRestarted   Type = "AgentRestarted"
	TypeDecisionRecorded Type = "DecisionRecorded"
)

// Event is a single entry in the session event log.
type Event struct {
	Type      Type           `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload,omitempty"`
}
