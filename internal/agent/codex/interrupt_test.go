package codex

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestInterrupt_noopWhenNotStarted(t *testing.T) {
	c := New(".")
	if err := c.Interrupt(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestInterrupt_noopWhenTurnIDNotYetKnown(t *testing.T) {
	c := New(".")
	c.turn.mu.Lock()
	c.turn.state = turnState{kind: turnInflightUnknownID}
	c.turn.mu.Unlock()
	err := c.Interrupt()
	if err != nil {
		t.Fatalf("Unexpected error, got %v", err)
	}
}

func TestInterrupt_sendsTurnInterruptWhenTurnActive(t *testing.T) {
	stdin := &strings.Builder{}
	stdoutR, stdoutW := io.Pipe()
	t.Cleanup(func() { _ = stdoutR.Close() })

	c := newWithIO(nopWriteCloser{stdin}, stdoutR, nil)
	c.turn.mu.Lock()
	c.turn.threadID = "t1"
	c.turn.state = turnState{kind: turnInflightUnknownID}
	c.turn.mu.Unlock()

	_, _ = io.WriteString(stdoutW, "{\"method\":\"turn/started\",\"params\":{\"threadId\":\"t1\",\"turn\":{\"id\":\"turn-1\"}}}\n")

	deadline := time.NewTimer(250 * time.Millisecond)
	defer deadline.Stop()
	for {
		c.turn.mu.Lock()
		state := c.turn.state
		c.turn.mu.Unlock()
		if state.kind == turnInflightKnownID && state.turnID == "turn-1" {
			break
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for active turn id; got state=%v", state.kind)
		default:
			time.Sleep(1 * time.Millisecond)
		}
	}

	if err := c.Interrupt(); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	_ = stdoutW.Close()

	got := stdin.String()
	if !strings.Contains(got, "\"method\":\"turn/interrupt\"") {
		t.Fatalf("expected turn/interrupt request, got %q", got)
	}
	if !strings.Contains(got, "\"threadId\":\"t1\"") || !strings.Contains(got, "\"turnId\":\"turn-1\"") {
		t.Fatalf("expected threadId and turnId in request, got %q", got)
	}
}
