package codex

import "github.com/trigosec/coderoom/internal/agent"

// Stream ID constructors for Codex item-scoped streams.
func outputStreamID(turnID, itemID string) agent.StreamID {
	return agent.StreamID("codex:output:" + turnID + ":" + itemID)
}

func reasoningStreamID(itemID string) agent.StreamID {
	return agent.StreamID("codex:reasoning:" + itemID)
}

func commandStreamID(itemID string) agent.StreamID {
	return agent.StreamID("codex:command:" + itemID)
}

func fileChangeStreamID(itemID string) agent.StreamID {
	return agent.StreamID("codex:fileChange:" + itemID)
}

// Fixed stream IDs for synthetic and log streams.
const (
	logStreamID         = agent.StreamID("codex:log")
	noticeRelayStreamID = agent.StreamID("codex:notice-relay")
	noticeTurnStreamID  = agent.StreamID("codex:notice-turn")
	// activeTurnStreamID is the turn-lifecycle anchor for regular (non-notice)
	// turns. Send() returns it; messageFromTurnCompleted closes it as the final
	// message of turn/completed, ensuring idle is never triggered before the
	// adapter has fully signalled turn completion.
	activeTurnStreamID = agent.StreamID("codex:active-turn")
)
