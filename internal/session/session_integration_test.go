//go:build integration

package session_test

import (
	"os"
	"testing"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/session"
)

// chanObserver forwards session events to a buffered channel.
type chanObserver struct {
	ch chan session.Event
}

func (o chanObserver) OnEvent(e session.Event) {
	select {
	case o.ch <- e:
	default:
	}
}

// drainUntil reads from ch until an event of the expected kind arrives or the
// timeout elapses. It discards events of other kinds so callers don't need to
// drain intermediate deltas before waiting for KindDone.
func drainUntil(t *testing.T, ch <-chan session.Event, want session.Kind, timeout time.Duration) session.Event {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("events channel closed while waiting for %q", want)
				return session.Event{}
			}
			if ev.Kind == want {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out after %s waiting for %q event", timeout, want)
			return session.Event{}
		}
	}
}

// TestSession_agentStopsCleanly verifies the full lifecycle of a session with a
// real Codex agent: invite → turn → stop → inert. The "no lingering process"
// guarantee is tested by asserting that Send fails after Stop.
func TestSession_agentStopsCleanly(t *testing.T) {
	cwd, _ := os.Getwd()

	events := make(chan session.Event, 128)
	var a *codex.Client
	s := session.New(
		session.WithObserver(chanObserver{ch: events}),
		session.WithAgentFactory(func(_ string) agent.Agent {
			a = codex.New(cwd)
			return a
		}),
	)

	if err := s.Execute(session.InviteCommand{
		Alias:      "ada",
		Role:       participant.RoleBuilder,
		Initiative: participant.InitiativeManual,
	}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	drainUntil(t, events, session.KindAgentStarted, 10*time.Second)

	if err := s.Execute(session.SharedSendCommand{
		Alias:      "ada",
		TextDirect: "What is 2+2? Reply with just the number.",
	}); err != nil {
		t.Fatalf("shared send: %v", err)
	}
	drainUntil(t, events, session.KindDone, 60*time.Second)

	if err := s.Execute(session.RemoveCommand{Alias: "ada"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	drainUntil(t, events, session.KindAgentStopped, 10*time.Second)

	if err := a.Send("hello after stop"); err == nil {
		t.Error("expected Send to fail after Stop, got nil")
	}
}
