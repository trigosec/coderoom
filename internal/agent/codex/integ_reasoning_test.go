//go:build integration

package codex_test

import (
	"os"
	"testing"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
)

// TestClientReasoningMessages verifies that reasoning notifications from a
// reasoning-capable model are delivered as Reasoning+ModeStream before the
// turn-end Output+ModeFlush.
func TestClientReasoningMessages(t *testing.T) {
	cwd, _ := os.Getwd()
	c := codex.New(cwd, codex.WithModel("gpt-5.2"), codex.WithObserver(wireObserverForTest(t)))
	startClient(t, c)

	anchorID, err := c.Send("Think through whether this condition is correct: `if (!items.length && isEnabled)`. What cases does it allow?")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	done := make(chan struct{})
	var reasoningCount int
	lastReasoningMode := make(map[agent.StreamID]agent.Mode)
	go func() {
		defer close(done)
		for {
			msg, err := c.Read()
			if err != nil {
				t.Errorf("Read: %v", err)
				return
			}
			switch c := msg.Content.(type) {
			case agent.Reasoning:
				if msg.Mode == agent.ModeStream {
					if c.Text == "" {
						t.Error("received Reasoning ModeStream with empty text")
					}
					reasoningCount++
				}
				lastReasoningMode[msg.StreamID] = msg.Mode
			case agent.Output:
				// Exit only on the anchor flush — the authoritative turn-end
				// signal. Per-item flushes from item/completed arrive earlier
				// and must not cut the loop short before reasoning streams flush.
				if msg.Mode == agent.ModeFlush && msg.StreamID == anchorID {
					return
				}
			default:
				t.Logf("unexpected message content type %T (mode: %v)", msg.Content, msg.Mode)
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for turn completion")
	}

	if reasoningCount == 0 {
		t.Error("expected at least one Reasoning+ModeStream before turn-end")
	}
	for id, mode := range lastReasoningMode {
		if mode != agent.ModeFlush {
			t.Errorf("reasoning stream %s never flushed", id)
		}
	}
}
