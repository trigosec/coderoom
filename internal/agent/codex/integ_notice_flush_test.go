//go:build integration

package codex_test

import (
	"os"
	"testing"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
)

func TestSendNotice_ackEmitsTurnFlush(t *testing.T) {
	cwd, _ := os.Getwd()
	c := codex.New(cwd, codex.WithObserver(wireObserverForTest(t)))
	startClient(t, c)

	if err := c.SendNotice("Store this in memory. Reply only with acknowledge true."); err != nil {
		t.Fatalf("SendNotice: %v", err)
	}

	deadline := time.After(60 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for notice turn flush")
		default:
		}

		msg, err := c.Read()
		if err != nil {
			t.Fatalf("Read: %v", err)
		}

		switch msg.Content.(type) {
		case agent.Output:
			if msg.Mode == agent.ModeStream {
				t.Fatalf("unexpected output delta during notice turn: stream=%q", msg.StreamID)
			}
			if msg.Mode == agent.ModeFlush && msg.StreamID == agent.StreamID("codex:notice-turn") {
				return
			}
		}
	}
}

