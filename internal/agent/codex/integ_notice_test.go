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
