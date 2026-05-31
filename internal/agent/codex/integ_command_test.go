//go:build integration

package codex_test

import (
	"testing"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
)

// TestClientCommandExecution verifies that shell command lifecycle notifications
// (item/started, item/commandExecution/outputDelta, item/completed) are
// delivered as Command messages with the correct modes and fields.
func TestClientCommandExecution(t *testing.T) {
	cwd := t.TempDir()
	c := codex.New(
		cwd,
		codex.WithAskForApprovalPolicy(codex.AskNever),
		codex.WithSandboxMode(codex.SandboxDangerFull),
		codex.WithObserver(wireObserverForTest(t)),
	)
	startClient(t, c)

	if _, err := c.Send(`Run the shell command: echo hello`); err != nil {
		t.Fatalf("Send: %v", err)
	}

	type cmdMsg struct {
		content  agent.Command
		mode     agent.Mode
		streamID agent.StreamID
	}

	var cmds []cmdMsg
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			msg, err := c.Read()
			if err != nil {
				t.Errorf("Read: %v", err)
				return
			}
			switch content := msg.Content.(type) {
			case agent.Command:
				cmds = append(cmds, cmdMsg{content, msg.Mode, msg.StreamID})
			case agent.Output:
				if msg.Mode == agent.ModeFlush {
					return
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for turn completion")
	}

	// Verify the lifecycle: item/started (Command set), item/completed (ExitCode
	// in ModeStream then zero-value ModeFlush), and at least one output delta.
	var sawStart, sawExitCode, sawFlush bool
	for _, m := range cmds {
		if m.mode == agent.ModeStream && m.content.Command != "" {
			sawStart = true
		}
		if m.mode == agent.ModeStream && m.content.ExitCode != nil {
			sawExitCode = true
			if *m.content.ExitCode != 0 {
				t.Errorf("expected exit code 0, got %d", *m.content.ExitCode)
			}
		}
		if m.mode == agent.ModeFlush {
			sawFlush = true
			if m.content.ExitCode != nil || m.content.Command != "" || m.content.Output != "" {
				t.Errorf("expected zero-value Command on ModeFlush, got %+v", m.content)
			}
		}
	}
	if !sawStart {
		t.Error("expected Command+ModeStream with Command field set (item/started), got none")
	}
	if !sawExitCode {
		t.Error("expected Command+ModeStream with ExitCode set (item/completed), got none")
	}
	if !sawFlush {
		t.Error("expected zero-value Command+ModeFlush after exit code, got none")
	}

	// Verify deltas accumulate to contain the expected output.
	outputByStream := make(map[agent.StreamID]string)
	for _, m := range cmds {
		if m.mode == agent.ModeStream {
			outputByStream[m.streamID] += m.content.Output
		}
	}
	var sawHello bool
	for _, out := range outputByStream {
		if out != "" {
			sawHello = true
			break
		}
	}
	if !sawHello {
		t.Error("expected at least one Command output delta, got none")
	}
}
