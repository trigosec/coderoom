package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/trigosec/coderoom/internal/agent"
	roomconfig "github.com/trigosec/coderoom/internal/config"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/promptlang"
	"github.com/trigosec/coderoom/internal/session"
	"github.com/trigosec/coderoom/internal/shell"
)

func TestLoop_sendsParticipantBeforeEvaluatingCondition(t *testing.T) {
	m, participantAgent := newLoopTestModel(t)
	runs := 0
	m.runShell = func(context.Context, string, string) shell.Result {
		runs++
		return shell.Result{Status: shell.StatusSuccess}
	}

	m = m.startLoop(testLoop(3))
	if runs != 0 {
		t.Fatal("condition ran before the participant turn")
	}
	if participantAgent.sendCalls != 1 || participantAgent.sent[0] != "make the tests pass" {
		t.Fatalf("initial participant prompts = %v, want plain loop prompt", participantAgent.sent)
	}

	m, cmd := completeLoopTurn(t, m, participantAgent)
	if cmd == nil {
		t.Fatal("participant completion did not schedule condition evaluation")
	}
	result := cmd().(loopConditionResultMsg)
	m, cmd = m.handleLoopConditionResult(result)
	if cmd != nil || m.activeLoop != nil {
		t.Fatal("successful condition did not finish loop")
	}
}

func TestLoop_alternatesConditionAndBoundedParticipantTurns(t *testing.T) {
	m, participantAgent := newLoopTestModel(t)
	conditionRuns := 0
	m.runShell = func(context.Context, string, string) shell.Result {
		conditionRuns++
		exitCode := 1
		return shell.Result{Status: shell.StatusFailure, ExitCode: &exitCode, Stdout: "tests failed"}
	}

	m = m.startLoop(testLoop(2))
	assertLoopCounts(t, participantAgent, conditionRuns, 1, 0)

	m, conditionCmd := completeLoopTurn(t, m, participantAgent)
	if conditionRuns != 0 {
		t.Fatal("condition evaluation ran synchronously")
	}

	m, nextCmd := applyLoopCondition(t, m, conditionCmd)
	if nextCmd != nil {
		t.Fatal("failed condition scheduled asynchronous work before the next participant turn")
	}
	assertLoopCounts(t, participantAgent, conditionRuns, 2, 1)
	if !strings.Contains(participantAgent.sent[1], "Condition command: /tests") {
		t.Fatalf("second participant prompt missing condition evidence: %q", participantAgent.sent[1])
	}

	m, conditionCmd = completeLoopTurn(t, m, participantAgent)
	m, nextCmd = applyLoopCondition(t, m, conditionCmd)
	if nextCmd != nil || m.activeLoop != nil {
		t.Fatal("failed final condition did not finish loop")
	}
	assertLoopCounts(t, participantAgent, conditionRuns, 2, 2)
}

func applyLoopCondition(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("participant completion did not schedule condition evaluation")
	}
	result := cmd().(loopConditionResultMsg)
	return m.handleLoopConditionResult(result)
}

func assertLoopCounts(t *testing.T, participantAgent *testAgent, conditionRuns, wantTurns, wantRuns int) {
	t.Helper()
	if participantAgent.sendCalls != wantTurns {
		t.Errorf("participant turns = %d, want %d", participantAgent.sendCalls, wantTurns)
	}
	if conditionRuns != wantRuns {
		t.Errorf("condition runs = %d, want %d", conditionRuns, wantRuns)
	}
}

func completeLoopTurn(t *testing.T, m Model, participantAgent *testAgent) (Model, tea.Cmd) {
	t.Helper()
	participantAgent.push(agent.Message{
		StreamID: testTurnAnchor,
		Mode:     agent.ModeFlush,
		Content:  agent.Output{},
	})
	idleEvent := pullUntilIdle(t, &m, "ada")
	return m.advanceLoopForEvent(idleEvent)
}

func newLoopTestModel(t *testing.T) (Model, *testAgent) {
	t.Helper()
	participantAgent := newTestAgent()
	sess := session.New(session.WithAgentFactory(func(*session.Session, roomconfig.ParticipantConfig) agent.Agent {
		return participantAgent
	}))
	t.Cleanup(sess.Shutdown)
	m := newTestModelWithSession(t, sess)
	inviteParticipant(t, sess, "ada", "#4ade80")
	m = pumpUntilAgentsStarted(t, m, "ada")
	defineLoopCondition(t, m.commands)
	return m, participantAgent
}

func defineLoopCondition(t *testing.T, registry *promptlang.Registry) {
	t.Helper()
	err := registry.Define(promptlang.CommandDefinition{
		Name: "tests",
		Body: promptlang.Shell{Program: "go test ./..."},
	})
	if err != nil {
		t.Fatalf("define loop condition: %v", err)
	}
}

func testLoop(maxTurns int) promptlang.Loop {
	return promptlang.Loop{
		Participant: "ada",
		Prompt:      "make the tests pass",
		Condition:   "tests",
		MaxTurns:    maxTurns,
	}
}

func pullUntilIdle(t *testing.T, m *Model, alias string) session.ParticipantStatusChanged {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		event := mustPullEvent(t, m, 2*time.Second)
		status, ok := event.(session.ParticipantStatusChanged)
		if ok && status.Alias == alias && status.To == participant.StatusIdle {
			return status
		}
	}
	t.Fatal("timed out waiting for participant completion")
	return session.ParticipantStatusChanged{}
}
