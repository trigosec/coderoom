package codex

import (
	"encoding/json"
	"fmt"

	"github.com/trigosec/coderoom/internal/agent"
)

// messageFromEnvelope converts a known Codex notification envelope into an
// agent.Message.
// Returns ok=false for unknown notifications (caller should discard and continue).
func messageFromEnvelope(msg rpcEnvelope) (agent.Message, bool, error) {
	switch msg.Method {
	case methodAgentDelta:
		var p deltaParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return agent.Message{}, false, fmt.Errorf("parse delta params: %w", err)
		}
		return agent.Message{Kind: agent.MessageDelta, Text: p.Delta}, true, nil
	case methodReasoningTextDelta, methodReasoningSummaryTextDelta:
		var p deltaParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return agent.Message{}, false, fmt.Errorf("parse reasoning delta params: %w", err)
		}
		return agent.Message{Kind: agent.MessageReasoning, Text: p.Delta}, true, nil
	case methodReasoningSummaryPartAdded:
		return agent.Message{Kind: agent.MessageReasoningContinue}, true, nil
	case methodTurnCompleted:
		return agent.Message{Kind: agent.MessageDone}, true, nil
	case methodTurnFailed:
		return agent.Message{}, false, fmt.Errorf("turn failed: %s", msg.Params)
	}
	return agent.Message{}, false, nil
}
