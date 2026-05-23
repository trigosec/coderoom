package codex

import "github.com/trigosec/coderoom/internal/agent"

// Stream ID constructors for turn-scoped streams.
func turnStreamID(turnID string) agent.StreamID {
	return agent.StreamID("codex:turn:" + turnID)
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
)
