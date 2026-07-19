//go:build integration

package session_test

import (
	"os"
	"testing"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
	roomconfig "github.com/trigosec/coderoom/internal/config"
	"github.com/trigosec/coderoom/internal/session"
)

// TestSession_agentStopsCleanly verifies the full lifecycle of a session with a
// real Codex agent: invite → turn → stop → inert. The "no lingering process"
// guarantee is tested by asserting that Send fails after Stop.
func TestSession_agentStopsCleanly(t *testing.T) {
	cwd, _ := os.Getwd()

	events := make(chan session.Event, 128)
	var a *codex.Client
	s := session.New(
		session.WithObserver(chanObserver{ch: events}),
		session.WithAgentFactory(func(_ *session.Session, _ roomconfig.ParticipantConfig) agent.Agent {
			a = codex.New(cwd)
			return a
		}),
	)
	t.Cleanup(s.Shutdown)

	if err := s.Execute(session.InviteCommand{
		Alias: "ada",
	}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	drainUntilType[session.AgentStarted](t, events, 10*time.Second)

	if err := s.Execute(session.SharedSendCommand{
		Alias:      "ada",
		TextDirect: "What is 2+2? Reply with just the number.",
	}); err != nil {
		t.Fatalf("shared send: %v", err)
	}
	drainUntilIdle(t, events, 60*time.Second)

	if err := s.Execute(session.RemoveCommand{Alias: "ada"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	drainUntilType[session.AgentStopped](t, events, 10*time.Second)

	if _, err := a.Send("hello after stop"); err == nil {
		t.Error("expected Send to fail after Stop, got nil")
	}
}
