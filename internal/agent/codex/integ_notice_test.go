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
// A notice turn must complete silently — no output deltas (Output+ModeStream) and
// no reasoning deltas (Reasoning+ModeStream) should be emitted before the probe
// turn's own turn-end flush (Output+ModeFlush).
//
// The probe turn (a follow-up Send) anchors the boundary: the relay path emits
// reasoning deltas followed by a turn-end flush with no preceding output delta,
// so a turn-end flush before any output delta is proof of a notice leak.
func TestSendNotice_silentAck(t *testing.T) {
	cwd, _ := os.Getwd()
	c := codex.New(cwd, codex.WithObserver(wireObserverForTest(t)))
	startClient(t, c)

	if _, err := c.SendNotice("The magic word is PICASSO."); err != nil {
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
		if _, err := c.Send("Reply with a single word: done"); err == nil {
			break
		}
		select {
		case <-noticeDeadline:
			t.Fatal("notice turn did not complete within 30s")
		default:
		}
	}

	// Drain until the probe turn's turn-end flush (Output+ModeFlush). The relay
	// path emits reasoning deltas and a turn-end flush with no output delta; the
	// probe turn emits an output delta before its turn-end flush. A turn-end flush
	// before any output delta means the notice turn leaked output.
	var seenDelta bool
	probeDeadline := time.After(30 * time.Second)
loop:
	for {
		select {
		case r := <-recvCh:
			if r.err != nil {
				break loop
			}
			switch c := r.msg.Content.(type) {
			case agent.Output:
				switch r.msg.Mode {
				case agent.ModeStream:
					seenDelta = true
				case agent.ModeFlush:
					// SendNotice emits a synthetic turn-end flush on a dedicated stream
					// so downstream consumers can treat it as a complete lifecycle.
					// It is not visible output and should not be treated as a leak.
					if r.msg.StreamID == agent.StreamID("codex:notice-turn") {
						continue
					}
					if !seenDelta {
						t.Errorf("notice turn leaked: turn-end without preceding output delta")
					}
					break loop
				}
			case agent.Reasoning:
				if r.msg.Mode == agent.ModeStream && !seenDelta {
					t.Errorf("notice turn leaked reasoning: %q", c.Text)
				}
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

	if _, err := c.SendNotice("our secret word is BANANA"); err != nil {
		t.Fatalf("SendNotice: %v", err)
	}

	// Poll until the notice turn completes, using the question as the probe.
	var probeAnchor agent.StreamID
	deadline := time.After(30 * time.Second)
	for {
		time.Sleep(250 * time.Millisecond)
		if id, err := c.Send("what is our secret word? Reply with just the word."); err == nil {
			probeAnchor = id
			break
		}
		select {
		case <-deadline:
			t.Fatal("notice turn did not complete within 30s")
		default:
		}
	}

	response := readResponse(t, c, probeAnchor, testTimeout)
	if !strings.Contains(strings.ToUpper(response), "BANANA") {
		t.Errorf("expected response to contain BANANA; got: %s", response)
	}
}
