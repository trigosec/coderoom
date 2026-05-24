package history

import (
	"testing"

	"github.com/trigosec/coderoom/internal/agent"
	rec "github.com/trigosec/coderoom/internal/ui/room/history/record"
)

func ptr(v int) *int { return &v }

func newCommandModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, "")
	m = m.SetSize(80, 40)
	return m
}

func TestHandleAgentMessage_commandStream_opensRecordWithCmdAndCwd(t *testing.T) {
	m := newCommandModel(t)
	m = m.HandleAgentMessage("bot", agent.Message{
		StreamID: "codex:command:c1",
		Mode:     agent.ModeStream,
		Content:  agent.Command{Command: "ls -la", Cwd: "/tmp"},
	})

	if len(m.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(m.records))
	}
	r := m.records[0]
	if r.Kind != rec.KindCommand {
		t.Errorf("expected KindCommand, got %v", r.Kind)
	}
	if r.Msg == nil {
		t.Fatal("expected record to carry Msg, got nil")
	}
	cmd, ok := r.Msg.Content.(agent.Command)
	if !ok {
		t.Fatalf("expected Command content, got %T", r.Msg.Content)
	}
	if cmd.Command != "ls -la" {
		t.Errorf("expected Command=%q, got %q", "ls -la", cmd.Command)
	}
	if cmd.Cwd != "/tmp" {
		t.Errorf("expected Cwd=%q, got %q", "/tmp", cmd.Cwd)
	}
	if cmd.ExitCode != nil {
		t.Errorf("expected nil ExitCode before seal, got %v", *cmd.ExitCode)
	}
}

func TestHandleAgentMessage_commandStream_accumulatesOutput(t *testing.T) {
	m := newCommandModel(t)
	m = m.HandleAgentMessage("bot", agent.Message{
		StreamID: "codex:command:c1",
		Mode:     agent.ModeStream,
		Content:  agent.Command{Command: "echo hi", Cwd: "/"},
	})
	m = m.HandleAgentMessage("bot", agent.Message{
		StreamID: "codex:command:c1",
		Mode:     agent.ModeStream,
		Content:  agent.Command{Output: "hi\n"},
	})

	r := m.records[0]
	if r.Msg == nil {
		t.Fatal("expected record to carry Msg, got nil")
	}
	cmd, ok := r.Msg.Content.(agent.Command)
	if !ok {
		t.Fatalf("expected Command content, got %T", r.Msg.Content)
	}
	if cmd.Output != "hi\n" {
		t.Errorf("expected output=%q, got %q", "hi\n", cmd.Output)
	}
	if len(m.streaming) != 1 {
		t.Errorf("expected 1 open stream after delta, got %d", len(m.streaming))
	}
}

func TestHandleAgentMessage_commandFlush_sealsExitCodeAndClearsStream(t *testing.T) {
	m := newCommandModel(t)
	m = m.HandleAgentMessage("bot", agent.Message{
		StreamID: "codex:command:c1",
		Mode:     agent.ModeStream,
		Content:  agent.Command{Command: "true", Cwd: "/"},
	})
	// item/completed emits ModeStream{ExitCode} then a zero-value ModeFlush.
	m = m.HandleAgentMessage("bot", agent.Message{
		StreamID: "codex:command:c1",
		Mode:     agent.ModeStream,
		Content:  agent.Command{ExitCode: ptr(0)},
	})
	m = m.HandleAgentMessage("bot", agent.Message{
		StreamID: "codex:command:c1",
		Mode:     agent.ModeFlush,
		Content:  agent.Command{},
	})

	if len(m.streaming) != 0 {
		t.Errorf("expected 0 open streams after flush, got %d", len(m.streaming))
	}
	r := m.records[0]
	if r.Msg == nil {
		t.Fatal("expected record to carry Msg, got nil")
	}
	cmd, ok := r.Msg.Content.(agent.Command)
	if !ok {
		t.Fatalf("expected Command content, got %T", r.Msg.Content)
	}
	if cmd.ExitCode == nil || *cmd.ExitCode != 0 {
		t.Errorf("expected ExitCode=0 after flush, got %v", cmd.ExitCode)
	}
}
