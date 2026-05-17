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

func TestRead_turnCompleted(t *testing.T) {
	stdout := bytes.NewBufferString("{\"method\":\"turn/completed\",\"params\":{}}\n")
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Kind != agent.MessageDone {
		t.Errorf("expected Kind=%q, got %q", agent.MessageDone, msg.Kind)
	}
}

func TestRead_delta(t *testing.T) {
	stdout := bytes.NewBufferString("{\"method\":\"item/agentMessage/delta\",\"params\":{\"delta\":\"hello\"}}\n")
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Kind != agent.MessageDelta || msg.Text != "hello" {
		t.Errorf("expected delta %q, got kind=%q text=%q", "hello", msg.Kind, msg.Text)
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
			"{\"method\":\"turn/completed\",\"params\":{}}\n",
	)
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Kind != agent.MessageDone {
		t.Errorf("expected Kind=%q, got %q", agent.MessageDone, msg.Kind)
	}
}

func TestRead_skipsUnknownNotifications(t *testing.T) {
	// Unknown notifications must be discarded; next known notification returned.
	stdout := bytes.NewBufferString(
		"{\"method\":\"turn/started\",\"params\":{}}\n" +
			"{\"method\":\"turn/completed\",\"params\":{}}\n",
	)
	c := newWithIO(t, nopWriteCloser{io.Discard}, stdout, nil)

	msg, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Kind != agent.MessageDone {
		t.Errorf("expected Kind=%q after skipping unknown notification, got %q", agent.MessageDone, msg.Kind)
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
	stdout := bytes.NewBufferString("{\"method\":\"turn/completed\",\"params\":{}}\n")
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
