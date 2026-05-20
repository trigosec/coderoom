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

// readResponse drains Read() until MessageDone and returns accumulated delta text.
func readResponse(t *testing.T, c *codex.Client, timeout time.Duration) string {
	t.Helper()
	var sb strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			msg, err := c.Read()
			if err != nil {
				return
			}
			switch msg.Kind {
			case agent.MessageDelta:
				sb.WriteString(msg.Text)
			case agent.MessageDone:
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("timed out after %s waiting for response", timeout)
	}
	return sb.String()
}

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

// TestSendNotice_silentAck verifies that SendNotice produces no visible output.
// A notice turn must complete silently — no MessageDelta and no MessageReasoning
// should be emitted before the probe turn's own MessageDone.
//
// The probe turn (a follow-up Send) anchors the boundary: the relay path always
// produces MessageReasoning+MessageDone with no preceding MessageDelta, so any
// MessageDone appearing before a MessageDelta is proof of a notice leak.
func TestSendNotice_silentAck(t *testing.T) {
	cwd, _ := os.Getwd()
	c := codex.New(cwd, codex.WithObserver(wireObserverForTest(t)))
	startClient(t, c)

	if err := c.SendNotice("The magic word is PICASSO."); err != nil {
		t.Fatalf("SendNotice: %v", err)
	}

	// Start reading immediately so notice-leaked messages are captured even
	// while we wait for the notice turn to end.
	type recv struct {
		msg agent.Message
		err error
	}
	recvCh := make(chan recv, 64)
	go func() {
		for {
			msg, err := c.Read()
			recvCh <- recv{msg, err}
			if err != nil {
				return
			}
		}
	}()

	// Poll until the notice turn completes (Send stops returning ErrTurnInProgress).
	noticeDeadline := time.After(30 * time.Second)
	for {
		time.Sleep(250 * time.Millisecond)
		if c.Send("Reply with a single word: done") == nil {
			break
		}
		select {
		case <-noticeDeadline:
			t.Fatal("notice turn did not complete within 30s")
		default:
		}
	}

	// Drain until the probe turn's MessageDone. The relay path emits
	// MessageReasoning+MessageDone with no MessageDelta; the probe turn emits
	// MessageDelta before MessageDone. A MessageDone before any MessageDelta
	// therefore means the notice turn leaked output.
	var seenDelta bool
	probeDeadline := time.After(30 * time.Second)
loop:
	for {
		select {
		case r := <-recvCh:
			if r.err != nil {
				break loop
			}
			switch r.msg.Kind {
			case agent.MessageDelta:
				seenDelta = true
			case agent.MessageReasoning:
				if !seenDelta {
					t.Errorf("notice turn leaked reasoning: %q", r.msg.Text)
				}
			case agent.MessageDone:
				if !seenDelta {
					t.Errorf("notice turn leaked: MessageDone without preceding MessageDelta")
				}
				break loop
			}
		case <-probeDeadline:
			t.Fatal("probe turn did not complete within 30s")
			return
		}
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

// TestSendNotice_contextRetained verifies that context delivered via SendNotice
// is retained: the agent recalls noticed information when asked directly in a
// subsequent turn.
func TestSendNotice_contextRetained(t *testing.T) {
	cwd, _ := os.Getwd()
	c := codex.New(cwd, codex.WithObserver(wireObserverForTest(t)))
	startClient(t, c)

	if err := c.SendNotice("our secret word is BANANA"); err != nil {
		t.Fatalf("SendNotice: %v", err)
	}

	// Poll until the notice turn completes, using the question as the probe.
	deadline := time.After(30 * time.Second)
	for {
		time.Sleep(250 * time.Millisecond)
		if c.Send("what is our secret word? Reply with just the word.") == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("notice turn did not complete within 30s")
		default:
		}
	}

	response := readResponse(t, c, 60*time.Second)
	if !strings.Contains(strings.ToUpper(response), "BANANA") {
		t.Errorf("expected response to contain BANANA; got: %s", response)
	}
}
