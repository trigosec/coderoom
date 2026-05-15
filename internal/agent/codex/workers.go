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
