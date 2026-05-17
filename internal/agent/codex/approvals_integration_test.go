//go:build integration

package codex_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
)

var logDir = flag.String("logdir", "", "directory to write Codex wire logs into (if empty, uses the test temp dir)")

type approvalListenerFunc func(req agent.ApprovalRequest) (agent.ApprovalDecision, error)

func (f approvalListenerFunc) Decide(req agent.ApprovalRequest) (agent.ApprovalDecision, error) {
	return f(req)
}

func TestApprovals_fileChange(t *testing.T) {
	cwd := t.TempDir()
	approvals := make(chan agent.ApprovalRequest, 16)
	l := approvalListenerFunc(func(req agent.ApprovalRequest) (agent.ApprovalDecision, error) {
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
	l := approvalListenerFunc(func(req agent.ApprovalRequest) (agent.ApprovalDecision, error) {
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

func wireObserverForTest(t *testing.T) codex.ProtocolObserver {
	t.Helper()
	dir := *logDir
	if dir == "" {
		dir = t.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("wirelog-dir mkdir: %v", err)
	}
	f, err := os.CreateTemp(dir, "codex-wire-*.log")
	if err != nil {
		t.Fatalf("wire log temp: %v", err)
	}
	t.Logf("codex wire log: %s", f.Name())
	t.Cleanup(func() { _ = f.Close() })
	return codex.NewLogObserver(f, "codex")
}
