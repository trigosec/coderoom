package session

// Kind identifies the type of a session event.
type Kind string

// Session event kinds.
const (
	KindAgentStarting Kind = "agent.starting"
	KindAgentStarted  Kind = "agent.started"
	KindAgentStopped  Kind = "agent.stopped"
	KindAgentCrashed  Kind = "agent.crashed"
	KindAgentLog      Kind = "agent.log" // diagnostic line from the agent process (e.g. stderr)

	KindBroadcast         Kind = "message.broadcast"          // message to all agents
	KindSharedSend        Kind = "message.shared"             // instruction to one agent, visible to all
	KindSharedNotice      Kind = "message.notice"             // context notice forwarded to a listener
	KindDelta             Kind = "message.delta"              // streaming text fragment
	KindReasoningDelta    Kind = "message.reasoning"          // streaming reasoning fragment
	KindReasoningContinue Kind = "message.reasoning.continue" // reasoning record boundary; seal current, next delta opens fresh
	KindDone              Kind = "message.done"               // turn complete
)

// Event is a runtime notification emitted by the session controller.
type Event struct {
	Kind  Kind
	Alias string // participant alias the event relates to
	Text  string // message text or delta fragment
}

// Observer receives session events. Implementations must be fast; a blocking
// observer will stall agent reader goroutines. If async processing is needed,
// buffer internally inside OnEvent.
type Observer interface {
	OnEvent(e Event)
}
