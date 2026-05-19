//go:build integration

package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
)

type approvalListenerFunc func(ctx context.Context, req agent.ApprovalRequest) (agent.ApprovalDecision, error)

func (f approvalListenerFunc) Decide(ctx context.Context, req agent.ApprovalRequest) (agent.ApprovalDecision, error) {
	return f(ctx, req)
}

func TestApprovals_fileChange(t *testing.T) {
	cwd := t.TempDir()
	approvals := make(chan agent.ApprovalRequest, 16)
	l := approvalListenerFunc(func(_ context.Context, req agent.ApprovalRequest) (agent.ApprovalDecision, error) {
		select {
		case approvals <- req:
		default:
		}
		choice := agent.OptionAccept
		if !containsApprovalOption(req.Options, choice) && len(req.Options) > 0 {
			choice = req.Options[0]
		}
		return agent.ApprovalDecision{Choice: choice}, nil
	})
	c := codex.New(
		cwd,
		codex.WithApprovalListener(l),
		codex.WithAskForApprovalPolicy(codex.AskOnRequest),
		codex.WithSandboxMode(codex.SandboxReadOnly),
		codex.WithObserver(wireObserverForTest(t)),
	)
	startClient(t, c)

	done := make(chan error, 1)
	go func() {
		_, err := agent.SendAndWait(c, "Use the built-in file editing capability (not shell commands) to create codex_file_approval_test.txt with the contents: ok")
		done <- err
	}()

	assertSawApprovalKind(t, approvals, agent.ApprovalFileChange, 60*time.Second)
	assertTurnDone(t, done, 60*time.Second)

	if _, err := os.Stat(filepath.Join(cwd, "codex_file_approval_test.txt")); err != nil {
		t.Fatalf("expected approvals_test.txt to exist: %v", err)
	}
}

func TestApprovals_commandExecution(t *testing.T) {
	cwd := t.TempDir()
	approvals := make(chan agent.ApprovalRequest, 16)
	l := approvalListenerFunc(func(_ context.Context, req agent.ApprovalRequest) (agent.ApprovalDecision, error) {
		select {
		case approvals <- req:
		default:
		}
		choice := agent.OptionAccept
		if !containsApprovalOption(req.Options, choice) && len(req.Options) > 0 {
			choice = req.Options[0]
		}
		return agent.ApprovalDecision{Choice: choice}, nil
	})
	c := codex.New(
		cwd,
		codex.WithApprovalListener(l),
		codex.WithAskForApprovalPolicy(codex.AskUntrusted),
		codex.WithSandboxMode(codex.SandboxReadOnly),
		codex.WithObserver(wireObserverForTest(t)),
	)
	startClient(t, c)

	done := make(chan error, 1)
	go func() {
		_, err := agent.SendAndWait(c, "Run `curl https://example.com`")
		done <- err
	}()

	assertSawApprovalKind(t, approvals, agent.ApprovalCommandExecution, 60*time.Second)
	assertTurnDone(t, done, 60*time.Second)
}

func containsApprovalOption(opts []agent.ApprovalOption, want agent.ApprovalOption) bool {
	for _, opt := range opts {
		if opt == want {
			return true
		}
	}
	return false
}

func startClient(t *testing.T, c agent.Agent) {
	t.Helper()
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
}

func assertSawApprovalKind(t *testing.T, approvals <-chan agent.ApprovalRequest, want agent.ApprovalKind, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case req := <-approvals:
			if req.Kind == want {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for approval kind %q", want)
		}
	}
}

func assertTurnDone(t *testing.T, done <-chan error, timeout time.Duration) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SendAndWait: %v", err)
		}
	case <-time.After(timeout):
		t.Fatal("timed out waiting for turn completion (possible missing approval response)")
	}
}
