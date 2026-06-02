package session

import (
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
)

// Kind identifies the type of a session event.
type Kind string

// Session event kinds.
const (
	KindAgentStarting Kind = "agent.starting"
	KindAgentStarted  Kind = "agent.started"
	KindAgentStopped  Kind = "agent.stopped"
	KindAgentCrashed  Kind = "agent.crashed"
	KindAgentLog      Kind = "agent.log"     // diagnostic line from the agent process (e.g. stderr)
	KindAgentMessage  Kind = "agent.message" // typed agent output; see Msg field

	KindParticipantStatusChanged Kind = "participant.status"

	KindBroadcast    Kind = "message.broadcast" // message sent to all agents
	KindSharedSend   Kind = "message.shared"    // instruction to one agent, visible to all
	KindSharedNotice Kind = "message.notice"    // context notice forwarded to a listener

	KindApprovalRequested Kind = "approval.requested" // approval request requiring user decision
	KindApprovalCleared   Kind = "approval.cleared"   // active approval prompt should be dismissed
)

// Event is a runtime notification emitted by the session controller.
type Event struct {
	Kind  Kind
	Alias string         // participant alias the event relates to
	Text  string         // for KindBroadcast, KindSharedSend, KindSharedNotice, KindAgentLog
	Msg   *agent.Message // for KindAgentMessage; nil for all other kinds

	ApprovalID  int64                  // for KindApprovalRequested, KindApprovalCleared
	ApprovalReq *agent.ApprovalRequest // for KindApprovalRequested

	StatusFrom participant.Status // for KindParticipantStatusChanged
	StatusTo   participant.Status // for KindParticipantStatusChanged
	Since      time.Time          // for KindParticipantStatusChanged; equals participant.Since after the transition
}

// Observer receives session events. Implementations must be fast; a blocking
// observer will stall agent reader goroutines. If async processing is needed,
// buffer internally inside OnEvent.
type Observer interface {
	OnEvent(e Event)
}
