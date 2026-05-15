package codex

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/linestream"
)

type workerFn = func(ctx context.Context, c *Client)

// readCodexErrWorker reads the Codex process stderr stream and emits each
// coalesced chunk as a log message on c.read.messages.
//
// This worker is long-lived and is expected to be started from Client.Start().
// It exits when ctx is canceled or when codex stderr closes.
func readCodexErrWorker(ctx context.Context, c *Client) {
	r := c.proc.codexErr
	for chunk := range linestream.BatchReader(r) {
		select {
		case <-ctx.Done():
			return
		case c.read.messages <- readMessage{msg: agent.Message{Kind: agent.MessageLog, Text: chunk}}:
		}
	}
}

// readCodexOut reads raw stdout lines, translates them to readMessage values,
// and sends meaningful ones on bufMessages.
func readCodexOutWorker(ctx context.Context, c *Client) {
	for {
		msg, err := rpcRead(c)
		if err != nil {
			if nonJSON, ok := isNonJSONStdoutLine(err); ok {
				readMsg := readMessage{msg: agent.Message{Kind: agent.MessageLog, Text: nonJSON.FormatLogLine()}}
				if !sendBufMessage(ctx, c, readMsg) {
					return
				}
				continue
			}
			readMsg := readMessage{err: err}
			sendBufMessage(ctx, c, readMsg)
			return
		}
		if msg.Method == "" {
			continue
		}
		c.noteNotification(msg)
		agentMsg, ok, err := translateNotification(msg)
		if err != nil {
			readMsg := readMessage{err: err}
			sendBufMessage(ctx, c, readMsg)
			return
		}
		if ok {
			readMsg := readMessage{msg: agentMsg}
			if !sendBufMessage(ctx, c, readMsg) {
				return
			}
		}
	}
}

func sendBufMessage(ctx context.Context, c *Client, msg readMessage) bool {
	select {
	case c.read.bufMessages <- msg:
		return true
	case <-ctx.Done():
		return false
	}
}

// bufferMessagesWorker bridges the stdout pipe to read.messages via an unbounded internal
// buffer. Memory grows if the consumer falls behind; acceptable for a local
// CLI tool where stdout throughput is bounded by the agent's output rate.
//
// bufferMessagesWorker does not close read.messages directly because stderr also writes to
// it. Start() closes read.messages after both workers exit.
func bufferMessagesWorker(ctx context.Context, c *Client) {
	var buf []readMessage
	for {
		if len(buf) == 0 {
			select {
			case r, ok := <-c.read.bufMessages:
				if !ok {
					return
				}
				buf = append(buf, r)
			case <-ctx.Done():
				return
			}
		} else {
			select {
			case r, ok := <-c.read.bufMessages:
				if !ok {
					drainMessageBuffer(c, buf)
					return
				}
				buf = append(buf, r)
			case c.read.messages <- buf[0]:
				buf = buf[1:]
			case <-ctx.Done():
				drainMessageBuffer(c, buf)
				return
			}
		}
	}
}

func drainMessageBuffer(c *Client, buf []readMessage) {
	for _, pending := range buf {
		select {
		case c.read.messages <- pending:
		default:
		}
	}
}

// translateNotification maps a known Codex notification to an agent.Message.
// Returns ok=false for unknown notifications (caller should discard and continue).
func translateNotification(msg rpcEnvelope) (agent.Message, bool, error) {
	switch msg.Method {
	case methodAgentDelta:
		var p deltaParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return agent.Message{}, false, fmt.Errorf("parse delta params: %w", err)
		}
		return agent.Message{Kind: agent.MessageDelta, Text: p.Delta}, true, nil
	case methodTurnCompleted:
		return agent.Message{Kind: agent.MessageDone}, true, nil
	case methodTurnFailed:
		return agent.Message{}, false, fmt.Errorf("turn failed: %s", msg.Params)
	}
	return agent.Message{}, false, nil
}
