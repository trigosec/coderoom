//go:build integration

package codex_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
)

// TestClientSingleTurn verifies basic communication with the Codex app-server.
func TestClientSingleTurn(t *testing.T) {
	cwd, _ := os.Getwd()
	c := codex.New(cwd, codex.WithObserver(wireObserverForTest(t)))
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	result, err := agent.SendAndWait(c, "What is 2 + 2?")
	if err != nil {
		t.Fatalf("SendAndWait: %v", err)
	}
	if !strings.Contains(result, "4") {
		t.Errorf("expected result to contain '4', got: %s", result)
	}
}

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

// TestClientContextPreservation verifies that context is maintained across
// turns within a single thread.
func TestClientContextPreservation(t *testing.T) {
	cwd, _ := os.Getwd()
	c := codex.New(cwd, codex.WithObserver(wireObserverForTest(t)))
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	if _, err := agent.SendAndWait(c, "What is 2 + 2?"); err != nil {
		t.Fatalf("first turn: %v", err)
	}
	// result is accumulated from item/agentMessage/delta notifications,
	// not the full JSON event, so contains is safe here.
	result, err := agent.SendAndWait(c, "Multiply that result by 3.")
	if err != nil {
		t.Fatalf("second turn: %v", err)
	}
	if !strings.Contains(result, "12") {
		t.Errorf("expected result to contain '12' (context preserved), got: %s", result)
	}
}
