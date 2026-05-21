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
		var p notificationParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return agent.Message{}, false, fmt.Errorf("parse agent delta params: %w", err)
		}
		return agent.Message{
			StreamID: turnStreamID(p.TurnID),
			Mode:     agent.ModeStream,
			Content:  agent.Output{Text: p.Delta},
		}, true, nil
	case methodReasoningTextDelta, methodReasoningSummaryTextDelta:
		var p notificationParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return agent.Message{}, false, fmt.Errorf("parse reasoning delta params: %w", err)
		}
		return agent.Message{
			StreamID: reasoningStreamID(p.ItemID),
			Mode:     agent.ModeStream,
			Content:  agent.Reasoning{Text: p.Delta},
		}, true, nil
	case methodReasoningSummaryPartAdded:
		var p notificationParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return agent.Message{}, false, fmt.Errorf("parse reasoning part added params: %w", err)
		}
		return agent.Message{
			StreamID: reasoningStreamID(p.ItemID),
			Mode:     agent.ModeFlush,
			Content:  agent.Reasoning{},
		}, true, nil
	case methodTurnCompleted:
		var p turnCompletedParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return agent.Message{}, false, fmt.Errorf("parse turn completed params: %w", err)
		}
		return agent.Message{
			StreamID: turnStreamID(p.Turn.ID),
			Mode:     agent.ModeFlush,
			Content:  agent.Output{},
		}, true, nil
	case methodTurnFailed:
		return agent.Message{}, false, fmt.Errorf("turn failed: %s", msg.Params)
	}
	return agent.Message{}, false, nil
}
