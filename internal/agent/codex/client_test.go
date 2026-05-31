package codex

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/trigosec/coderoom/internal/agent"
)

// nopWriteCloser wraps a Writer with a no-op Close for use as a stdin stub.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func line(s string) string { return s + "\n" }

func turnCompletedLine(itemIDs ...string) string {
	items := make([]string, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		items = append(items, `{"type":"agentMessage","id":"`+itemID+`"}`)
	}
	return line(`{"method":"turn/completed","params":{"threadId":"th1","turn":{"id":"t1","status":"completed","items":[` + strings.Join(items, ",") + `]}}}`)
}

// newWithIO constructs a Client with pre-wired I/O and starts the readStdout
// goroutine, mirroring what Start() does after the handshake. Used in tests.
func newWithIO(t *testing.T, stdin io.WriteCloser, stdout io.Reader, obs ProtocolObserver) *Client {
	if obs == nil {
		obs = noopObserver{}
	}
	c := &Client{proc: newProc("test")}
	c.proc.codexIn = stdin
	c.proc.codexOut = bufio.NewReader(stdout)
	c.proc.codexErr = bytes.NewBuffer(nil)
	c.rpc.obs = obs
	c.initRead()
	c.lifecycle.ctx, c.lifecycle.cancelFn = context.WithCancel(context.Background()) // #nosec: G118
	t.Cleanup(c.lifecycle.cancelFn)
	c.initWorkers()
	return c
}

func TestCodexArgs_default(t *testing.T) {
	t.Setenv("CODEX_VERSION_OVERRIDE", "")
	args := codexArgs("", "")
	if args[1] != "@openai/codex" {
		t.Errorf("expected @openai/codex, got %q", args[1])
	}
}

func TestCodexArgs_override(t *testing.T) {
	t.Setenv("CODEX_VERSION_OVERRIDE", "0.99.0")
	args := codexArgs("", "")
	if args[1] != "@openai/codex@0.99.0" {
		t.Errorf("expected @openai/codex@0.99.0, got %q", args[1])
	}
}

func TestCodexArgs_modelNotInArgs(t *testing.T) {
	// Model is passed via thread/start JSON params, not as a CLI flag.
	t.Setenv("CODEX_VERSION_OVERRIDE", "")
	args := codexArgs("", "")
	for _, a := range args {
		if a == "--model" {
			t.Errorf("--model must not appear in CLI args, got %v", args)
		}
	}
}

func TestRead_turnCompleted(t *testing.T) {
	// Current Codex protocol sends items as "notLoaded" (empty); only the anchor
	// flush is emitted. Per-item flushes come from item/completed (agentMessage).
	stdout := bytes.NewBufferString(turnCompletedLine())
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush for turn/completed, got mode=%v", msg.Mode)
	}
	if msg.StreamID != activeTurnStreamID {
		t.Fatalf("expected anchor stream ID %q, got %q", activeTurnStreamID, msg.StreamID)
	}
}

func TestRead_delta(t *testing.T) {
	wire := line(`{"method":"item/agentMessage/delta","params":{"itemId":"msg1","turnId":"turn1","delta":"hello"}}`)
	stdout := bytes.NewBufferString(wire)
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.StreamID != agent.StreamID("codex:output:turn1:msg1") {
		t.Fatalf("expected item-scoped stream ID, got %q", msg.StreamID)
	}
	out, ok := msg.Content.(agent.Output)
	if !ok || out.Text != "hello" {
		t.Errorf("expected Output{hello}, got mode=%v content=%T", msg.Mode, msg.Content)
	}
}

func TestRead_reasoningTextDelta(t *testing.T) {
	wire := line(`{"method":"item/reasoning/textDelta","params":{"delta":"let me think","contentIndex":0,"itemId":"i1","threadId":"t1","turnId":"u1"}}`)
	stdout := bytes.NewBufferString(wire)
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := msg.Content.(agent.Reasoning)
	if !ok || r.Text != "let me think" {
		t.Errorf("expected Reasoning{let me think}, got mode=%v content=%T", msg.Mode, msg.Content)
	}
}

func TestRead_reasoningSummaryTextDelta(t *testing.T) {
	wire := line(`{"method":"item/reasoning/summaryTextDelta","params":{"delta":"summary fragment","summaryIndex":0,"itemId":"i1","threadId":"t1","turnId":"u1"}}`)
	stdout := bytes.NewBufferString(wire)
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := msg.Content.(agent.Reasoning)
	if !ok || r.Text != "summary fragment" {
		t.Errorf("expected Reasoning{summary fragment}, got mode=%v content=%T", msg.Mode, msg.Content)
	}
}

func TestRead_reasoningSummaryPartAdded_continue(t *testing.T) {
	wire := line(`{"method":"item/reasoning/summaryPartAdded","params":{"summaryIndex":0,"itemId":"i1","threadId":"t1","turnId":"u1"}}`)
	stdout := bytes.NewBufferString(wire)
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush for summaryPartAdded, got mode=%v", msg.Mode)
	}
}

func TestRead_itemStarted_commandExecution(t *testing.T) {
	item := `{"type":"commandExecution","id":"cmd1","command":"ls -la","cwd":"/tmp","status":"inProgress","commandActions":[]}`
	params := `{"turnId":"t1","threadId":"th1","startedAtMs":0,"item":` + item + `}`
	stdout := bytes.NewBufferString(`{"method":"item/started","params":` + params + `}` + "\n")
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmd, ok := msg.Content.(agent.Command)
	if !ok {
		t.Fatalf("expected Command content, got %T", msg.Content)
	}
	if msg.Mode != agent.ModeStream {
		t.Errorf("expected ModeStream, got mode=%v", msg.Mode)
	}
	if cmd.Command != "ls -la" || cmd.Cwd != "/tmp" {
		t.Errorf("unexpected command fields: command=%q cwd=%q", cmd.Command, cmd.Cwd)
	}
}

func TestRead_itemStarted_fileChange(t *testing.T) {
	item := `{"type":"fileChange","id":"fc1","status":"inProgress","changes":[{"path":"a.txt","diff":"+hi\n","kind":{"type":"add"}}]}`
	params := `{"turnId":"t1","threadId":"th1","startedAtMs":0,"item":` + item + `}`
	stdout := bytes.NewBufferString(`{"method":"item/started","params":` + params + `}` + "\n")
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fc, ok := msg.Content.(agent.FileChangeSet)
	if !ok {
		t.Fatalf("expected FileChangeSet content, got %T", msg.Content)
	}
	if msg.Mode != agent.ModeStream {
		t.Errorf("expected ModeStream, got mode=%v", msg.Mode)
	}
	if fc.Status != agent.ToolStatusInProgress {
		t.Errorf("expected status %q, got %q", agent.ToolStatusInProgress, fc.Status)
	}
	if len(fc.Changes) != 1 || fc.Changes[0].Path != "a.txt" || fc.Changes[0].ChangeKind != "add" {
		t.Errorf("unexpected file change payload: %+v", fc)
	}
}

func TestRead_itemStarted_nonCommand_skipped(t *testing.T) {
	// item/started for a non-commandExecution type must be silently skipped.
	// turn/completed always emits the anchor flush (activeTurnStreamID), so we
	// consume that first and then expect EOF on the next Read.
	nonCmd := `{"type":"agentMessage","id":"a1","text":"hi"}`
	params := `{"turnId":"t1","threadId":"th1","startedAtMs":0,"item":` + nonCmd + `}`
	stdout := bytes.NewBufferString(
		`{"method":"item/started","params":` + params + `}` + "\n" +
			turnCompletedLine(),
	)
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	// Consume the anchor flush emitted by turn/completed.
	msg, err := c.Read()
	if err != nil {
		t.Fatalf("expected anchor flush, got error: %v", err)
	}
	if msg.StreamID != activeTurnStreamID || msg.Mode != agent.ModeFlush {
		t.Fatalf("expected anchor flush (streamID=%q, mode=ModeFlush), got streamID=%q mode=%v", activeTurnStreamID, msg.StreamID, msg.Mode)
	}

	// No more messages; expect EOF.
	if _, err := c.Read(); err == nil {
		t.Fatal("expected EOF after skip-only turn completion, got nil")
	}
}

func TestRead_commandExecutionOutputDelta(t *testing.T) {
	wire := line(`{"method":"item/commandExecution/outputDelta","params":{"itemId":"cmd1","turnId":"t1","threadId":"th1","delta":"hello\n"}}`)
	stdout := bytes.NewBufferString(wire)
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmd, ok := msg.Content.(agent.Command)
	if !ok {
		t.Fatalf("expected Command content, got %T", msg.Content)
	}
	if msg.Mode != agent.ModeStream {
		t.Errorf("expected ModeStream, got mode=%v", msg.Mode)
	}
	if cmd.Output != "hello\n" {
		t.Errorf("unexpected output: %q", cmd.Output)
	}
}

func TestRead_fileChangePatchUpdated(t *testing.T) {
	wire := line(`{"method":"item/fileChange/patchUpdated","params":{"itemId":"fc1","turnId":"t1","threadId":"th1","changes":[{"path":"b.txt","diff":"@@\n","kind":{"type":"update","move_path":null}}]}}`)
	stdout := bytes.NewBufferString(wire)
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fc, ok := msg.Content.(agent.FileChangeSet)
	if !ok {
		t.Fatalf("expected FileChangeSet content, got %T", msg.Content)
	}
	if msg.Mode != agent.ModeStream {
		t.Errorf("expected ModeStream, got mode=%v", msg.Mode)
	}
	if len(fc.Changes) != 1 || fc.Changes[0].Path != "b.txt" || fc.Changes[0].ChangeKind != "update" {
		t.Errorf("unexpected file change payload: %+v", fc)
	}
}

func TestRead_itemCompleted_commandExecution(t *testing.T) {
	item := `{"type":"commandExecution","id":"cmd1","command":"ls","cwd":"/tmp","status":"completed","commandActions":[],"exitCode":0}`
	params := `{"turnId":"t1","threadId":"th1","completedAtMs":0,"item":` + item + `}`
	stdout := bytes.NewBufferString(`{"method":"item/completed","params":` + params + `}` + "\n")
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	// item/completed emits two messages: ModeStream with ExitCode, then zero-value ModeFlush.
	assertCommandExitCodeStream(t, c, 0)
	assertCommandZeroFlush(t, c)
}

func TestRead_itemCompleted_fileChange(t *testing.T) {
	item := `{"type":"fileChange","id":"fc1","status":"completed","changes":[{"path":"c.txt","diff":"-old\n+new\n","kind":{"type":"update","move_path":null}}]}`
	params := `{"turnId":"t1","threadId":"th1","completedAtMs":0,"item":` + item + `}`
	stdout := bytes.NewBufferString(`{"method":"item/completed","params":` + params + `}` + "\n")
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("Read (file change stream): %v", err)
	}
	fc, ok := msg.Content.(agent.FileChangeSet)
	if !ok {
		t.Fatalf("expected FileChangeSet content, got %T", msg.Content)
	}
	if msg.Mode != agent.ModeStream {
		t.Errorf("expected ModeStream, got %v", msg.Mode)
	}
	if fc.Status != agent.ToolStatusCompleted {
		t.Errorf("expected status %q, got %q", agent.ToolStatusCompleted, fc.Status)
	}
	if len(fc.Changes) != 1 || fc.Changes[0].Path != "c.txt" {
		t.Errorf("unexpected changes: %+v", fc.Changes)
	}

	msg, err = c.Read()
	if err != nil {
		t.Fatalf("Read (file change flush): %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush, got %v", msg.Mode)
	}
	if _, ok := msg.Content.(agent.FileChangeSet); !ok {
		t.Fatalf("expected FileChangeSet flush content, got %T", msg.Content)
	}
}

func assertCommandExitCodeStream(t *testing.T, c *Client, wantExit int) {
	t.Helper()
	msg, err := c.Read()
	if err != nil {
		t.Fatalf("Read (exit code stream): %v", err)
	}
	cmd, ok := msg.Content.(agent.Command)
	if !ok {
		t.Fatalf("expected Command content, got %T", msg.Content)
	}
	if msg.Mode != agent.ModeStream {
		t.Errorf("expected ModeStream, got %v", msg.Mode)
	}
	if cmd.ExitCode == nil {
		t.Error("ExitCode is nil on exit code stream message")
	} else if *cmd.ExitCode != wantExit {
		t.Errorf("expected ExitCode=%d, got %d", wantExit, *cmd.ExitCode)
	}
}

func assertCommandZeroFlush(t *testing.T, c *Client) {
	t.Helper()
	msg, err := c.Read()
	if err != nil {
		t.Fatalf("Read (flush): %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush, got %v", msg.Mode)
	}
	flush, ok := msg.Content.(agent.Command)
	if !ok {
		t.Fatalf("expected Command content on flush, got %T", msg.Content)
	}
	if flush != (agent.Command{}) {
		t.Errorf("expected zero-value Command on flush, got %+v", flush)
	}
}

func TestRead_turnFailed(t *testing.T) {
	stdout := bytes.NewBufferString("{\"method\":\"turn/failed\",\"params\":{}}\n")
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	_, err := c.Read()
	if err == nil {
		t.Fatal("expected error for turn/failed, got nil")
	}
}

func TestRead_skipsResponseLines(t *testing.T) {
	// Response line (ID-bearing, no method) must be skipped; known notification returned.
	stdout := bytes.NewBufferString(
		"{\"id\":1,\"result\":{}}\n" +
			turnCompletedLine("msg1"),
	)
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush for turn/completed, got mode=%v", msg.Mode)
	}
}

func TestRead_skipsUnknownNotifications(t *testing.T) {
	// Unknown notifications must be discarded; next known notification returned.
	stdout := bytes.NewBufferString(
		"{\"method\":\"turn/started\",\"params\":{}}\n" +
			turnCompletedLine("msg1"),
	)
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Mode != agent.ModeFlush {
		t.Errorf("expected ModeFlush after skipping unknown notification, got mode=%v", msg.Mode)
	}
}

func TestRead_returnsErrorOnEOF(t *testing.T) {
	c := newWithIO(t, nopWriteCloser{io.Discard}, bytes.NewBuffer(nil), nil)

	_, err := c.Read()
	if err == nil {
		t.Fatal("expected error on empty reader, got nil")
	}
}

func TestRead_observerReceivesCalled(t *testing.T) {
	stdout := bytes.NewBufferString(turnCompletedLine("msg1"))
	received := make(chan string, 1)
	obs := &testObserver{onReceive: func(msg string) { received <- msg }}
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, obs)

	if _, err := c.Read(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case msg := <-received:
		if !strings.Contains(msg, "turn/completed") {
			t.Errorf("observer got unexpected msg: %q", msg)
		}
	default:
		t.Fatal("expected 1 OnReceive call, got 0")
	}
}

func TestWriteRequest_observerSendCalled(t *testing.T) {
	var buf bytes.Buffer
	var sent []string
	obs := &testObserver{onSend: func(msg string) { sent = append(sent, msg) }}
	c := newWithIO(t, nopWriteCloser{&buf}, bytes.NewBuffer(nil), obs)

	if err := rpcWrite(c, methodTurnStart, turnStartParams{ThreadID: "t1"}); err != nil {
		t.Fatalf("rpcWrite: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("expected 1 OnSend call, got %d", len(sent))
	}
	if !strings.Contains(sent[0], methodTurnStart) {
		t.Errorf("observer got unexpected msg: %q", sent[0])
	}
	if strings.Contains(buf.String(), "\n") && !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("written JSON should end with newline delimiter")
	}
}

type testObserver struct {
	onSend    func(string)
	onReceive func(string)
}

func (o *testObserver) OnSend(msg string) {
	if o.onSend != nil {
		o.onSend(msg)
	}
}

func (o *testObserver) OnReceive(msg string) {
	if o.onReceive != nil {
		o.onReceive(msg)
	}
}
