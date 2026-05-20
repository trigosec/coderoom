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

	case methodTurnCompleted:
		return c.handleNoticeCompleted(ctx)

	case methodTurnFailed:
		c.notice.mu.Lock()
		prevState := c.notice.state
		c.notice.state = noticeIdle
		c.notice.buf.Reset()
		c.notice.mu.Unlock()
		if prevState == noticeRelaying {
			// Reasoning deltas were already emitted; emit done to return the
			// participant to idle rather than leaving it stuck in working.
			return outcomeOf(sendBufMessage(ctx, c, readMessage{msg: agent.Message{Kind: agent.MessageDone}}))
		}
		return noticeContinue

	default:
		// Approval requests and other protocol messages pass through unfiltered.
		return noticeUnhandled
	}
}

func (c *Client) handleNoticeDelta(ctx context.Context, msg rpcEnvelope) noticeOutcome {
	var p deltaParams
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
		return outcomeOf(sendBufMessage(ctx, c, readMessage{msg: agent.Message{Kind: agent.MessageReasoning, Text: p.Delta}}))

	case noticeBuffering:
		c.notice.buf.WriteString(p.Delta)
		c.notice.mu.Unlock()
		return noticeContinue

	case noticeRelaying:
		c.notice.mu.Unlock()
		return outcomeOf(sendBufMessage(ctx, c, readMessage{msg: agent.Message{Kind: agent.MessageReasoning, Text: p.Delta}}))

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
		return noticeContinue

	case noticeBuffering:
		var r struct {
			Acknowledge bool `json:"acknowledge"`
		}
		if err := json.Unmarshal([]byte(buf), &r); err == nil && r.Acknowledge {
			return noticeContinue
		}
		// Not acknowledged: replay as reasoning then emit done.
		if o := outcomeOf(sendBufMessage(ctx, c, readMessage{msg: agent.Message{Kind: agent.MessageReasoning, Text: buf}})); o == noticeExit {
			return noticeExit
		}
		return outcomeOf(sendBufMessage(ctx, c, readMessage{msg: agent.Message{Kind: agent.MessageDone}}))

	case noticeRelaying:
		return outcomeOf(sendBufMessage(ctx, c, readMessage{msg: agent.Message{Kind: agent.MessageDone}}))

	default:
		return noticeUnhandled
	}
}

// outcomeOf converts the bool returned by sendBufMessage into a noticeOutcome.
func outcomeOf(ok bool) noticeOutcome {
	if ok {
		return noticeContinue
	}
	return noticeExit
}
