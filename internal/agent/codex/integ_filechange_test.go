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

func TestFileChange_emitsFileChangeSetMessages(t *testing.T) {
	cwd := t.TempDir()

	l := approvalListenerFunc(func(_ context.Context, req agent.ApprovalRequest) (agent.ApprovalDecision, error) {
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

	const timeout = testTimeout
	if _, err := c.Send("Use the built-in file editing capability (not shell commands) to create codex_filechange_stream_test.txt with the contents: ok"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	sawStream := false
	sawFlush := false
	fileChangeStreams := make(map[agent.StreamID]struct{})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			msg, err := c.Read()
			if err != nil {
				t.Errorf("Read: %v", err)
				return
			}
			if _, ok := msg.Content.(agent.FileChangeSet); ok {
				switch msg.Mode {
				case agent.ModeStream:
					sawStream = true
					fileChangeStreams[msg.StreamID] = struct{}{}
				case agent.ModeFlush:
					if _, ok := fileChangeStreams[msg.StreamID]; ok {
						sawFlush = true
					}
				default:
				}
			}
			if _, ok := msg.Content.(agent.Output); ok && msg.Mode == agent.ModeFlush {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("timed out after %s waiting for file change + turn completion (stream=%v flush=%v)", timeout, sawStream, sawFlush)
	}

	if !sawStream {
		t.Fatal("expected to see at least one FileChangeSet ModeStream message")
	}
	if !sawFlush {
		t.Fatal("expected to see a FileChangeSet ModeFlush message for a previously seen stream")
	}

	if _, err := os.Stat(filepath.Join(cwd, "codex_filechange_stream_test.txt")); err != nil {
		t.Fatalf("expected codex_filechange_stream_test.txt to exist: %v", err)
	}
}
