package codex

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/trigosec/coderoom/internal/agent"
)

type noticeState uint8

const (
	noticeIdle      noticeState = iota
	noticePending               // SendNotice called; awaiting turn/started
	noticeActive                // turn/started received; awaiting first delta
	noticeBuffering             // first delta started with '{'; buffering until turn/completed
	noticeRelaying              // first delta did not start with '{'; relaying as reasoning
)

// noticeOutcome is returned by interceptNotice and its helpers.
type noticeOutcome uint8

const (
	noticeUnhandled noticeOutcome = iota // not a notice turn; caller should process normally
	noticeContinue                       // handled; keep reading
	noticeExit                           // handled; exit worker (context cancelled)
)

// noticeContextPrefix is prepended to every notice prompt. It instructs the
// model to respond with only {"acknowledge":true} so the response can be
// silently discarded. Any JSON response containing "acknowledge":true is
// treated as compliant; extra fields are intentionally accepted.
const noticeContextPrefix = "[CONTEXT UPDATE — respond only with {\"acknowledge\":true}]\n\n"

// noticeOutputSchema is passed as outputSchema in the turn/start request.
// It constrains the agent message to the acknowledgment shape at the Codex
// protocol level, complementing the prompt instruction for models that honour
// structured output.
var noticeOutputSchema = json.RawMessage(`{"type":"object","properties":{"acknowledge":{"type":"boolean","const":true}},"required":["acknowledge"],"additionalProperties":false}`)

// interceptNotice is called from handleStdoutEnvelope after turn state has
// been updated. noticeUnhandled means the caller should process the envelope
// normally; noticeContinue and noticeExit mean the filter took ownership.
func (c *Client) interceptNotice(ctx context.Context, msg rpcEnvelope) noticeOutcome {
	c.notice.mu.Lock()
	state := c.notice.state
	c.notice.mu.Unlock()

	if state == noticeIdle {
		return noticeUnhandled
	}

	switch msg.Method {
	case methodTurnStarted:
		// noticePending transitions to noticeActive on the next turn/started.
		// No turn ID verification is needed: SendNotice enforces ErrTurnInProgress
		// so only one turn can be in flight at a time, meaning the next
		// turn/started is always ours.
		c.notice.mu.Lock()
		if c.notice.state == noticePending {
			c.notice.state = noticeActive
		}
		c.notice.mu.Unlock()
		return noticeContinue

	case methodAgentDelta, methodReasoningTextDelta, methodReasoningSummaryTextDelta:
		return c.handleNoticeDelta(ctx, msg)

	case methodReasoningSummaryPartAdded:
		// Boundary marker during a notice turn — part of the buffered or relayed
		// response; discard either way.
		return noticeContinue

	case methodItemCompleted:
		// Suppress item/completed for agentMessage items: the delta was intercepted
		// so no output stream was opened; forwarding the flush would produce a
		// spurious "stream not tracked" error. Non-agentMessage items pass through.
		return suppressNoticeAgentMessageItemCompleted(msg)

	case methodTurnCompleted:
		return c.handleNoticeCompleted(ctx)

	case methodTurnFailed:
		return c.handleNoticeFailed(ctx)

	default:
		// Approval requests and other protocol messages pass through unfiltered.
		return noticeUnhandled
	}
}

func (c *Client) handleNoticeDelta(ctx context.Context, msg rpcEnvelope) noticeOutcome {
	var p notificationParams
	if err := json.Unmarshal(msg.Params, &p); err != nil || p.Delta == "" {
		return noticeContinue
	}

	// Reasoning deltas (thinking summaries) are always silently discarded during
	// a notice turn. The {-heuristic and acknowledgment check only apply to the
	// agent message, so reasoning must not trigger the noticeRelaying path and
	// swallow subsequent agent message deltas.
	if msg.Method == methodReasoningTextDelta || msg.Method == methodReasoningSummaryTextDelta {
		return noticeContinue
	}

	c.notice.mu.Lock()
	state := c.notice.state

	switch state {
	case noticeActive:
		trimmed := strings.TrimLeft(p.Delta, " \t\r\n")
		if strings.HasPrefix(trimmed, "{") {
			c.notice.state = noticeBuffering
			c.notice.buf.WriteString(p.Delta)
			c.notice.mu.Unlock()
			return noticeContinue
		}
		c.notice.state = noticeRelaying
		c.notice.mu.Unlock()
		return relayDelta(ctx, c, p.Delta)

	case noticeBuffering:
		c.notice.buf.WriteString(p.Delta)
		c.notice.mu.Unlock()
		return noticeContinue

	case noticeRelaying:
		c.notice.mu.Unlock()
		return relayDelta(ctx, c, p.Delta)

	default:
		c.notice.mu.Unlock()
		return noticeUnhandled
	}
}

func (c *Client) handleNoticeCompleted(ctx context.Context) noticeOutcome {
	c.notice.mu.Lock()
	state := c.notice.state
	buf := c.notice.buf.String()
	c.notice.state = noticeIdle
	c.notice.buf.Reset()
	c.notice.mu.Unlock()

	switch state {
	case noticeActive:
		// No deltas at all — treat as acknowledgment.
		return c.emitNoticeTurnFlush(ctx)

	case noticeBuffering:
		var r struct {
			Acknowledge bool `json:"acknowledge"`
		}
		if err := json.Unmarshal([]byte(buf), &r); err == nil && r.Acknowledge {
			return c.emitNoticeTurnFlush(ctx)
		}
		// Not acknowledged: replay as reasoning then flush both streams.
		if o := relayDelta(ctx, c, buf); o == noticeExit {
			return noticeExit
		}
		return relayAndTurnFlush(ctx, c)

	case noticeRelaying:
		// Flush reasoning stream then emit turn-level flush.
		return relayAndTurnFlush(ctx, c)

	default:
		return noticeUnhandled
	}
}

func (c *Client) emitNoticeTurnFlush(ctx context.Context) noticeOutcome {
	rm := readMessage{
		msg: agent.Message{
			StreamID: noticeTurnStreamID,
			Mode:     agent.ModeFlush,
			Content:  agent.Output{},
		},
	}
	return outcomeOf(sendBufMessage(ctx, c, rm))
}

// relayDelta emits a single reasoning-stream delta fragment.
func relayDelta(ctx context.Context, c *Client, text string) noticeOutcome {
	rm := readMessage{
		msg: agent.Message{
			StreamID: noticeRelayStreamID,
			Mode:     agent.ModeStream,
			Content:  agent.Reasoning{Text: text},
		},
	}
	return outcomeOf(sendBufMessage(ctx, c, rm))
}

// relayAndTurnFlush flushes the reasoning relay stream and then emits the
// turn-level output flush so the participant returns to idle.
func relayAndTurnFlush(ctx context.Context, c *Client) noticeOutcome {
	rm := readMessage{
		msg: agent.Message{
			StreamID: noticeRelayStreamID,
			Mode:     agent.ModeFlush,
			Content:  agent.Reasoning{},
		},
	}
	if o := outcomeOf(sendBufMessage(ctx, c, rm)); o == noticeExit {
		return noticeExit
	}
	rm = readMessage{
		msg: agent.Message{
			StreamID: noticeTurnStreamID,
			Mode:     agent.ModeFlush,
			Content:  agent.Output{},
		},
	}
	return outcomeOf(sendBufMessage(ctx, c, rm))
}

// handleNoticeFailed handles methodTurnFailed during a notice turn.
func (c *Client) handleNoticeFailed(ctx context.Context) noticeOutcome {
	c.notice.mu.Lock()
	prevState := c.notice.state
	c.notice.state = noticeIdle
	c.notice.buf.Reset()
	c.notice.mu.Unlock()
	if prevState == noticeRelaying {
		// Reasoning deltas were already emitted; flush reasoning stream then
		// emit a turn-level flush so the participant returns to idle.
		return relayAndTurnFlush(ctx, c)
	}
	return c.emitNoticeTurnFlush(ctx)
}

func suppressNoticeAgentMessageItemCompleted(msg rpcEnvelope) noticeOutcome {
	var p itemLifecycleParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		return noticeUnhandled
	}
	var kind itemKind
	if err := json.Unmarshal(p.Item, &kind); err != nil {
		return noticeUnhandled
	}
	if kind.Type == "agentMessage" {
		return noticeContinue
	}
	return noticeUnhandled
}

// outcomeOf converts the bool returned by sendBufMessage into a noticeOutcome.
func outcomeOf(ok bool) noticeOutcome {
	if ok {
		return noticeContinue
	}
	return noticeExit
}
