package codex

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
)

// nopWriteCloser wraps a Writer with a no-op Close for use as a stdin stub.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// newWithIO constructs a Client with pre-wired I/O and starts the readStdout
// goroutine, mirroring what Start() does after the handshake. Used in tests.
func newWithIO(stdin io.WriteCloser, stdout io.Reader, obs ProtocolObserver) *Client {
	if obs == nil {
		obs = noopObserver{}
	}
	c := &Client{proc: newProc("test")}
	c.proc.codexIn = stdin
	c.proc.codexOut = bufio.NewReader(stdout)
	c.proc.codexErr = bytes.NewBuffer(nil)
	c.rpc.obs = obs
	c.initRead()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		c.readStdout()
	}()
	go func() {
		defer wg.Done()
		c.readStderr()
	}()
	go func() {
		wg.Wait()
		close(c.read.events)
	}()
	return c
}

func TestCodexArgs_default(t *testing.T) {
	t.Setenv("CODEX_VERSION_OVERRIDE", "")
	args := codexArgs()
	if args[1] != "@openai/codex" {
		t.Errorf("expected @openai/codex, got %q", args[1])
	}
}

func TestCodexArgs_override(t *testing.T) {
	t.Setenv("CODEX_VERSION_OVERRIDE", "0.99.0")
	args := codexArgs()
	if args[1] != "@openai/codex@0.99.0" {
		t.Errorf("expected @openai/codex@0.99.0, got %q", args[1])
	}
}

func TestRead_turnCompleted(t *testing.T) {
	stdout := bytes.NewBufferString("{\"method\":\"turn/completed\",\"params\":{}}\n")
	c := newWithIO(nopWriteCloser{io.Discard}, stdout, nil)

	ev, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ev.Done {
		t.Errorf("expected Done=true")
	}
}

func TestRead_delta(t *testing.T) {
	stdout := bytes.NewBufferString("{\"method\":\"item/agentMessage/delta\",\"params\":{\"delta\":\"hello\"}}\n")
	c := newWithIO(nopWriteCloser{io.Discard}, stdout, nil)

	ev, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Delta != "hello" {
		t.Errorf("expected delta %q, got %q", "hello", ev.Delta)
	}
}

func TestRead_turnFailed(t *testing.T) {
	stdout := bytes.NewBufferString("{\"method\":\"turn/failed\",\"params\":{}}\n")
	c := newWithIO(nopWriteCloser{io.Discard}, stdout, nil)

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
	c := newWithIO(nopWriteCloser{io.Discard}, stdout, nil)

	ev, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ev.Done {
		t.Errorf("expected Done=true")
	}
}

func TestRead_skipsUnknownNotifications(t *testing.T) {
	// Unknown notifications must be discarded; next known notification returned.
	stdout := bytes.NewBufferString(
		"{\"method\":\"turn/started\",\"params\":{}}\n" +
			"{\"method\":\"turn/completed\",\"params\":{}}\n",
	)
	c := newWithIO(nopWriteCloser{io.Discard}, stdout, nil)

	ev, err := c.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ev.Done {
		t.Errorf("expected Done=true after skipping unknown notification")
	}
}

func TestRead_returnsErrorOnEOF(t *testing.T) {
	c := newWithIO(nopWriteCloser{io.Discard}, bytes.NewBuffer(nil), nil)

	_, err := c.Read()
	if err == nil {
		t.Fatal("expected error on empty reader, got nil")
	}
}

func TestRead_observerReceivesCalled(t *testing.T) {
	stdout := bytes.NewBufferString("{\"method\":\"turn/completed\",\"params\":{}}\n")
	received := make(chan string, 1)
	obs := &testObserver{onReceive: func(msg string) { received <- msg }}
	c := newWithIO(nopWriteCloser{io.Discard}, stdout, obs)

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
	c := newWithIO(nopWriteCloser{&buf}, bytes.NewBuffer(nil), obs)

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
