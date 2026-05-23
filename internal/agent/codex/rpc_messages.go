package codex

import (
	"encoding/json"
	"fmt"

	"github.com/trigosec/coderoom/internal/agent"
)

// messageFromEnvelope converts a known Codex notification envelope into zero
// or more agent.Messages. Unknown notifications return an empty slice (caller
// should discard and continue). item/completed for a commandExecution returns
// two messages: ModeStream carrying ExitCode, then a zero-value ModeFlush.
func messageFromEnvelope(msg rpcEnvelope) ([]agent.Message, error) {
	switch msg.Method {
	case methodAgentDelta:
		return oneMsg(messageFromAgentDelta(msg.Params))
	case methodReasoningTextDelta, methodReasoningSummaryTextDelta:
		return oneMsg(messageFromReasoningDelta(msg.Params))
	case methodReasoningSummaryPartAdded:
		return oneMsg(messageFromReasoningSummaryPartAdded(msg.Params))
	case methodTurnCompleted:
		return oneMsg(messageFromTurnCompleted(msg.Params))
	case methodTurnFailed:
		return nil, fmt.Errorf("turn failed: %s", msg.Params)
	case methodItemStarted:
		return oneMsg(messageFromItemStarted(msg.Params))
	case methodCommandExecutionOutputDelta:
		return oneMsg(messageFromCommandOutputDelta(msg.Params))
	case methodFileChangePatchUpdated:
		return oneMsg(messageFromFileChangePatchUpdated(msg.Params))
	case methodItemCompleted:
		return messageFromItemCompleted(msg.Params)
	}
	return nil, nil
}

// oneMsg wraps the (Message, bool, error) triple returned by per-method helpers
// into the []Message slice used by messageFromEnvelope.
func oneMsg(m agent.Message, ok bool, err error) ([]agent.Message, error) {
	if err != nil || !ok {
		return nil, err
	}
	return []agent.Message{m}, nil
}

func messageFromAgentDelta(raw json.RawMessage) (agent.Message, bool, error) {
	var p notificationParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return agent.Message{}, false, fmt.Errorf("parse agent delta params: %w", err)
	}
	return agent.Message{
		StreamID: turnStreamID(p.TurnID),
		Mode:     agent.ModeStream,
		Content:  agent.Output{Text: p.Delta},
	}, true, nil
}

func messageFromReasoningDelta(raw json.RawMessage) (agent.Message, bool, error) {
	var p notificationParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return agent.Message{}, false, fmt.Errorf("parse reasoning delta params: %w", err)
	}
	return agent.Message{
		StreamID: reasoningStreamID(p.ItemID),
		Mode:     agent.ModeStream,
		Content:  agent.Reasoning{Text: p.Delta},
	}, true, nil
}

func messageFromReasoningSummaryPartAdded(raw json.RawMessage) (agent.Message, bool, error) {
	var p notificationParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return agent.Message{}, false, fmt.Errorf("parse reasoning part added params: %w", err)
	}
	return agent.Message{
		StreamID: reasoningStreamID(p.ItemID),
		Mode:     agent.ModeFlush,
		Content:  agent.Reasoning{},
	}, true, nil
}

func messageFromTurnCompleted(raw json.RawMessage) (agent.Message, bool, error) {
	var p turnCompletedParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return agent.Message{}, false, fmt.Errorf("parse turn completed params: %w", err)
	}
	return agent.Message{
		StreamID: turnStreamID(p.Turn.ID),
		Mode:     agent.ModeFlush,
		Content:  agent.Output{},
	}, true, nil
}

func messageFromItemStarted(raw json.RawMessage) (agent.Message, bool, error) {
	var p itemLifecycleParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return agent.Message{}, false, fmt.Errorf("parse item/started: %w", err)
	}
	var kind itemKind
	if err := json.Unmarshal(p.Item, &kind); err != nil {
		return agent.Message{}, false, fmt.Errorf("parse item/started item kind: %w", err)
	}
	switch kind.Type {
	case "commandExecution":
		var item commandExecutionItem
		if err := json.Unmarshal(p.Item, &item); err != nil {
			return agent.Message{}, false, fmt.Errorf("parse item/started commandExecution: %w", err)
		}
		return agent.Message{
			StreamID: commandStreamID(item.ID),
			Mode:     agent.ModeStream,
			Content:  agent.Command{Command: item.Command, Cwd: item.Cwd},
		}, true, nil
	case "fileChange":
		var item fileChangeItem
		if err := json.Unmarshal(p.Item, &item); err != nil {
			return agent.Message{}, false, fmt.Errorf("parse item/started fileChange: %w", err)
		}
		return agent.Message{
			StreamID: fileChangeStreamID(item.ID),
			Mode:     agent.ModeStream,
			Content: agent.FileChangeSet{
				Status:  agent.ToolStatus(item.Status),
				Changes: fileChangesFromWire(item.Changes),
			},
		}, true, nil
	default:
		return agent.Message{}, false, nil
	}
}

func messageFromCommandOutputDelta(raw json.RawMessage) (agent.Message, bool, error) {
	var p notificationParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return agent.Message{}, false, fmt.Errorf("parse command output delta: %w", err)
	}
	return agent.Message{
		StreamID: commandStreamID(p.ItemID),
		Mode:     agent.ModeStream,
		Content:  agent.Command{Output: p.Delta},
	}, true, nil
}

func messageFromFileChangePatchUpdated(raw json.RawMessage) (agent.Message, bool, error) {
	var p fileChangePatchUpdatedParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return agent.Message{}, false, fmt.Errorf("parse file change patch updated: %w", err)
	}
	return agent.Message{
		StreamID: fileChangeStreamID(p.ItemID),
		Mode:     agent.ModeStream,
		Content:  agent.FileChangeSet{Changes: fileChangesFromWire(p.Changes)},
	}, true, nil
}

// messageFromItemCompleted returns two messages for commandExecution items:
// a ModeStream carrying ExitCode, followed by a zero-value ModeFlush.
// Non-commandExecution items return an empty slice.
func messageFromItemCompleted(raw json.RawMessage) ([]agent.Message, error) {
	var p itemLifecycleParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parse item/completed: %w", err)
	}
	var kind itemKind
	if err := json.Unmarshal(p.Item, &kind); err != nil {
		return nil, fmt.Errorf("parse item/completed item kind: %w", err)
	}
	switch kind.Type {
	case "commandExecution":
		var item commandExecutionItem
		if err := json.Unmarshal(p.Item, &item); err != nil {
			return nil, fmt.Errorf("parse item/completed commandExecution: %w", err)
		}
		streamID := commandStreamID(item.ID)
		completed := agent.CommandWithOverrideOutput(item.AggregatedOutput)
		completed.ExitCode = item.ExitCode
		return []agent.Message{
			{StreamID: streamID, Mode: agent.ModeStream, Content: completed},
			{StreamID: streamID, Mode: agent.ModeFlush, Content: agent.Command{}},
		}, nil
	case "fileChange":
		var item fileChangeItem
		if err := json.Unmarshal(p.Item, &item); err != nil {
			return nil, fmt.Errorf("parse item/completed fileChange: %w", err)
		}
		streamID := fileChangeStreamID(item.ID)
		return []agent.Message{
			{
				StreamID: streamID,
				Mode:     agent.ModeStream,
				Content: agent.FileChangeSet{
					Status:  agent.ToolStatus(item.Status),
					Changes: fileChangesFromWire(item.Changes),
				},
			},
			{StreamID: streamID, Mode: agent.ModeFlush, Content: agent.FileChangeSet{}},
		}, nil
	case "reasoning":
		return []agent.Message{
			{StreamID: reasoningStreamID(kind.ID), Mode: agent.ModeFlush, Content: agent.Reasoning{}},
		}, nil
	default:
		return nil, nil
	}
}

func fileChangesFromWire(changes []fileUpdateChange) []agent.FileChange {
	if len(changes) == 0 {
		return nil
	}
	out := make([]agent.FileChange, 0, len(changes))
	for _, ch := range changes {
		out = append(out, agent.FileChange{
			Path:       ch.Path,
			Diff:       ch.Diff,
			ChangeKind: ch.Kind.Type,
		})
	}
	return out
}
