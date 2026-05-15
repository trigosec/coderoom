package codex

import (
	"context"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/linestream"
)

type workerFn = func(ctx context.Context, c *Client)

// readCodexErrWorker reads the Codex process stderr stream and emits each
// coalesced chunk as a Log event on c.read.events.
//
// This worker is long-lived and is expected to be started from Client.Start().
// It exits when ctx is canceled or when codex stderr closes.
func readCodexErrWorker(ctx context.Context, c *Client) {
	r := c.proc.codexErr
	for chunk := range linestream.BatchReader(r) {
		select {
		case <-ctx.Done():
			return
		case c.read.events <- readEvent{ev: agent.Event{Log: chunk}}:
		}
	}
}

// readCodexOut reads raw stdout lines, translates them to readEvent values, and
// sends meaningful ones on bufEvents.
func readCodexOutWorker(ctx context.Context, c *Client) {
	for {
		msg, err := rpcRead(c)
		if err != nil {
			if nonJSON, ok := isNonJSONStdoutLine(err); ok {
				readEv := readEvent{ev: agent.Event{Log: nonJSON.FormatLogLine()}}
				if !sendBufEvent(ctx, c, readEv) {
					return
				}
				continue
			}
			readEv := readEvent{err: err}
			sendBufEvent(ctx, c, readEv)
			return
		}
		if msg.Method == "" {
			continue
		}
		c.noteNotification(msg)
		ev, ok, err := translateNotification(msg)
		if err != nil {
			readEv := readEvent{err: err}
			sendBufEvent(ctx, c, readEv)
			return
		}
		if ok {
			readEv := readEvent{ev: ev}
			if !sendBufEvent(ctx, c, readEv) {
				return
			}
		}
	}
}

func sendBufEvent(ctx context.Context, c *Client, ev readEvent) bool {
	select {
	case c.read.bufEvents <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

// bufferEventsWorker bridges the stdout pipe to read.events via an unbounded internal
// buffer. Memory grows if the consumer falls behind; acceptable for a local
// CLI tool where stdout throughput is bounded by the agent's output rate.
//
// bufferEventsWorker does not close read.events directly because stderr also writes to
// it. Start() closes read.events after both workers exit.
func bufferEventsWorker(ctx context.Context, c *Client) {
	var buf []readEvent
	for {
		if len(buf) == 0 {
			select {
			case r, ok := <-c.read.bufEvents:
				if !ok {
					return
				}
				buf = append(buf, r)
			case <-ctx.Done():
				return
			}
		} else {
			select {
			case r, ok := <-c.read.bufEvents:
				if !ok {
					drainEventBuffer(c, buf)
					return
				}
				buf = append(buf, r)
			case c.read.events <- buf[0]:
				buf = buf[1:]
			case <-ctx.Done():
				drainEventBuffer(c, buf)
				return
			}
		}
	}
}

func drainEventBuffer(c *Client, buf []readEvent) {
	for _, pending := range buf {
		select {
		case c.read.events <- pending:
		default:
		}
	}
}
