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
// reasoning-capable model are delivered as MessageReasoning before MessageDone.
func TestClientReasoningMessages(t *testing.T) {
	cwd, _ := os.Getwd()
	c := codex.New(cwd, codex.WithModel("gpt-5.2"), codex.WithObserver(wireObserverForTest(t)))
	startClient(t, c)

	if err := c.Send("Think through whether this condition is correct: `if (!items.length && isEnabled)`. What cases does it allow?"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	done := make(chan struct{})
	var reasoningCount int
	go func() {
		defer close(done)
		for {
			msg, err := c.Read()
			if err != nil {
				t.Errorf("Read: %v", err)
				return
			}
			switch msg.Kind {
			case agent.MessageReasoning:
				if msg.Text == "" {
					t.Error("received MessageReasoning with empty text")
				}
				reasoningCount++
			case agent.MessageReasoningContinue:
				// boundary between summary parts — expected, no action needed
			case agent.MessageDone:
				return
			default:
				t.Logf("unexpected message kind %q (text: %q)", msg.Kind, msg.Text)
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("timed out waiting for turn completion")
	}

	if reasoningCount == 0 {
		t.Error("expected at least one MessageReasoning before MessageDone")
	}
}
