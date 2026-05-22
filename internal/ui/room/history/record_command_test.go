package history

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/trigosec/coderoom/internal/agent"
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
	if r.Kind != RecordKindCommand {
		t.Errorf("expected RecordKindCommand, got %v", r.Kind)
	}
	if r.Cmd != "ls -la" {
		t.Errorf("expected Cmd=%q, got %q", "ls -la", r.Cmd)
	}
	if r.Cwd != "/tmp" {
		t.Errorf("expected Cwd=%q, got %q", "/tmp", r.Cwd)
	}
	if r.ExitCode != nil {
		t.Errorf("expected nil ExitCode before seal, got %v", *r.ExitCode)
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
	if r.Body != "hi\n" {
		t.Errorf("expected body=%q, got %q", "hi\n", r.Body)
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
	if r.ExitCode == nil || *r.ExitCode != 0 {
		t.Errorf("expected ExitCode=0 after flush, got %v", r.ExitCode)
	}
}

func TestRenderCommand_headerOnly(t *testing.T) {
	r := Record{Kind: RecordKindCommand, Alias: "bot", Cmd: "ls", Cwd: "/tmp"}
	out := ansi.Strip(renderCommand(r, 80, func(string) string { return "" }))
	if !strings.HasPrefix(out, "● bot:\n\n  $ ls") {
		t.Errorf("expected header starting with participant prefix and command line, got %q", out)
	}
}

func TestRenderCommand_withOutputAndExitCode(t *testing.T) {
	r := Record{
		Kind: RecordKindCommand, Alias: "bot",
		Cmd:      "echo hi",
		Cwd:      "/",
		Body:     "hi\n",
		ExitCode: ptr(0),
	}
	out := ansi.Strip(renderCommand(r, 80, func(string) string { return "" }))
	if !strings.Contains(out, "echo hi") {
		t.Errorf("expected command in output, got %q", out)
	}
	if !strings.Contains(out, "\n\n  hi") {
		t.Errorf("expected output preview in render, got %q", out)
	}
	if strings.Contains(out, "Ctrl+O history") {
		t.Errorf("expected no navigation hint when nothing is hidden, got %q", out)
	}
	if !strings.Contains(out, "exit 0") {
		t.Errorf("expected exit code in render, got %q", out)
	}
}

func TestRenderCommand_outputPreview_showsTopThreeLinesAndHint(t *testing.T) {
	r := Record{
		Kind:     RecordKindCommand,
		Alias:    "bot",
		Cmd:      "echo stuff",
		Cwd:      "/",
		Body:     "a\nb\nc\nd\n",
		ExitCode: ptr(0),
	}
	out := ansi.Strip(renderCommand(r, 80, func(string) string { return "" }))
	if !strings.Contains(out, "\n\n  a\n  b\n  c") {
		t.Errorf("expected top 3 output lines in preview, got %q", out)
	}
	if strings.Contains(out, "\n  d") {
		t.Errorf("expected preview to omit remaining lines, got %q", out)
	}
	if !strings.Contains(out, "(+1 more; Ctrl+O history, Ctrl+G open transcript)") {
		t.Errorf("expected navigation hint for more lines, got %q", out)
	}
}

func TestRenderCommand_longCmd_truncatesToSingleLine(t *testing.T) {
	r := Record{Kind: RecordKindCommand, Alias: "bot", Cmd: "abcdefghij", Cwd: "/tmp"}
	out := ansi.Strip(renderCommand(r, 10, func(string) string { return "" }))
	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected header + blank + command line, got %q", out)
	}
	if strings.Contains(lines[2], "\n") {
		t.Fatalf("expected single command line, got %q", lines[2])
	}
	if !strings.Contains(lines[2], "…") {
		t.Errorf("expected truncated command to include ellipsis, got %q", lines[2])
	}
}

func TestRenderCommandLine_longCmdWrapsWithoutRepeatingDollarPrefix(t *testing.T) {
	// "  $ " is 4 columns wide; with width=10, contentWidth=6.
	out := renderCommandLine("  $ ", "abcdefghij", 10)
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapped output to span multiple lines, got %q", out)
	}
	if lines[0] != "  $ abcdef" {
		t.Errorf("unexpected first line, got %q", lines[0])
	}
	if lines[1] != "    ghij" {
		t.Errorf("unexpected continuation indent/content, got %q", lines[1])
	}
	for i, line := range lines[1:] {
		if strings.HasPrefix(line, "  $ ") {
			t.Errorf("continuation line %d should not repeat command prefix, got %q", i+1, line)
		}
	}
}

func TestRenderCommandTranscript_indentsAllOutputLines(t *testing.T) {
	r := Record{
		Kind:     RecordKindCommand,
		Alias:    "bot",
		Cmd:      "echo hi",
		Cwd:      "/",
		Body:     "line1\nline2\nline3\n",
		ExitCode: ptr(0),
	}
	out := ansi.Strip(renderCommandTranscript(r, func(string) string { return "" }))
	if !strings.Contains(out, "\n\n  line1\n  line2\n  line3") {
		t.Errorf("expected all output lines to be indented, got %q", out)
	}
}
