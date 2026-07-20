package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/promptlang"
	"github.com/trigosec/coderoom/internal/shell"
	"github.com/trigosec/coderoom/internal/ui/room/history/record"
)

func TestHandleSubmit_shellRunsAsynchronouslyAndRecordsResult(t *testing.T) {
	m := makeReadyModel(t)
	called := false
	exitCode := 7
	m.runShell = func(_ context.Context, cwd, program string) shell.Result {
		called = true
		if cwd != "." {
			t.Errorf("cwd = %q, want current model cwd", cwd)
		}
		if program != `echo "hello world" | false` {
			t.Errorf("program = %q, want raw shell program", program)
		}
		return shell.Result{
			Status:   shell.StatusFailure,
			ExitCode: &exitCode,
			Stdout:   "standard output",
			Stderr:   "standard error",
			Err:      errors.New("runner failure"),
		}
	}

	next, cmd := m.handleSubmit(`/shell echo "hello world" | false`)
	if called {
		t.Fatal("shell runner blocked submission instead of returning a command")
	}
	if cmd == nil {
		t.Fatal("expected asynchronous shell command")
	}
	if !hasRecord(next, record.KindUserInput, "/shell") {
		t.Fatal("expected shell input to be echoed before execution")
	}

	msg := cmd()
	if !called {
		t.Fatal("expected Bubble Tea command to call shell runner")
	}
	updated, _ := next.Update(msg)
	m = updated.(Model)
	command := shellCommandRecord(t, m)
	assertShellCommand(t, command)
}

func TestShellExecutionStopsWithUILifetime(t *testing.T) {
	tests := []struct {
		name string
		stop func(Model, context.CancelFunc)
	}{
		{"parent context", func(_ Model, cancel context.CancelFunc) { cancel() }},
		{"model close", func(m Model, _ context.CancelFunc) { m.Close() }},
		{"quit command", func(m Model, _ context.CancelFunc) {
			_, _ = m.executeUIAction(promptlang.Quit{})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertShellStopsWithUI(t, tt.stop)
		})
	}
}

func TestModelCloseWaitsForShellCompletion(t *testing.T) {
	m := New(context.Background(), newTestSession(t), ".")
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	m.runShell = func(ctx context.Context, _, _ string) shell.Result {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
		return shell.Result{Status: shell.StatusCancelled, Err: ctx.Err()}
	}

	commandDone := make(chan struct{})
	cmd := m.executeShell("long-running")
	go func() {
		_ = cmd()
		close(commandDone)
	}()
	<-started

	closeDone := make(chan struct{})
	go func() {
		m.Close()
		close(closeDone)
	}()
	<-cancelled
	select {
	case <-closeDone:
		t.Fatal("Model.Close returned before shell completion")
	default:
	}

	close(release)
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Model.Close did not return after shell completion")
	}
	<-commandDone
}

func assertShellStopsWithUI(t *testing.T, stop func(Model, context.CancelFunc)) {
	t.Helper()
	parent, cancelParent := context.WithCancel(context.Background())
	m := New(parent, newTestSession(t), ".")
	t.Cleanup(m.Close)
	started := make(chan struct{})
	m.runShell = func(ctx context.Context, _, _ string) shell.Result {
		close(started)
		<-ctx.Done()
		return shell.Result{Status: shell.StatusCancelled, Err: ctx.Err()}
	}

	resultCh := make(chan tea.Msg, 1)
	cmd := m.executeShell("long-running")
	go func() { resultCh <- cmd() }()
	<-started
	stop(m, cancelParent)

	select {
	case raw := <-resultCh:
		msg, ok := raw.(shellResultMsg)
		if !ok {
			t.Fatalf("message = %T, want shellResultMsg", raw)
		}
		if msg.result.Status != shell.StatusCancelled {
			t.Errorf("status = %q, want cancelled", msg.result.Status)
		}
		if !errors.Is(msg.result.Err, context.Canceled) {
			t.Errorf("error = %v, want context cancellation", msg.result.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shell execution survived UI shutdown")
	}
}

func shellCommandRecord(t *testing.T, m Model) agent.Command {
	t.Helper()
	for _, rec := range m.room.HistoryRecords() {
		if rec.Kind != record.KindCommand {
			continue
		}
		if rec.Alias != shellRecordAlias {
			t.Errorf("command alias = %q, want %q", rec.Alias, shellRecordAlias)
		}
		if rec.Msg == nil {
			t.Fatal("command record has no backing message")
		}
		command, ok := rec.Msg.Content.(agent.Command)
		if !ok {
			t.Fatalf("command content = %T, want agent.Command", rec.Msg.Content)
		}
		return command
	}
	t.Fatal("expected canonical command record")
	return agent.Command{}
}

func assertShellCommand(t *testing.T, command agent.Command) {
	t.Helper()
	if command.Command != `echo "hello world" | false` {
		t.Errorf("command = %q, want submitted program", command.Command)
	}
	if command.ExitCode == nil || *command.ExitCode != 7 {
		t.Errorf("exit code = %v, want 7", command.ExitCode)
	}
	for _, text := range []string{"status: failure", "stdout:\nstandard output", "stderr:\nstandard error", "error:\n"} {
		if !strings.Contains(command.Output, text) {
			t.Errorf("command output missing %q: %q", text, command.Output)
		}
	}
}
