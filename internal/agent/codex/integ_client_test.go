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

func TestClientOutputFlushesMatchSeenOutputStreams(t *testing.T) {
	cwd, _ := os.Getwd()
	c := codex.New(cwd, codex.WithObserver(wireObserverForTest(t)))
	startClient(t, c)

	anchorID, err := c.Send("Reply with exactly: hello")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	open := make(map[agent.StreamID]struct{})
	open[anchorID] = struct{}{}
	sawFlush := false
	deadline := time.After(testTimeout)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for all output streams to flush")
		default:
		}

		msg, err := c.Read()
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		out, ok := msg.Content.(agent.Output)
		if !ok {
			continue
		}
		switch msg.Mode {
		case agent.ModeStream:
			if out.Text == "" {
				continue
			}
			open[msg.StreamID] = struct{}{}
		case agent.ModeFlush:
			sawFlush = true
			if _, ok := open[msg.StreamID]; !ok {
				t.Fatalf("received output flush for unseen stream %q", msg.StreamID)
			}
			delete(open, msg.StreamID)
			if sawFlush && len(open) == 0 {
				return
			}
		}
	}
}
