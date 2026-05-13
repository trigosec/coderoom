// Package codex implements the Agent interface for the Codex CLI app-server.
// Communication uses JSON-RPC 2.0 over stdio (newline-delimited JSON).
// See docs/design/pkg-agent-codex.md for the full design rationale.
package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/linestream"
)

// readResult carries the outcome of a single stdout notification.
type readResult struct {
	ev  agent.Event
	err error
}

// Client implements agent.Agent for the Codex app-server.
// Calls must not be made concurrently, except Stop() which may be called
// from another goroutine to interrupt a blocked Read().
type Client struct {
	// proc holds the OS process and stdio pipes.
	proc struct {
		cwd string
		cmd *exec.Cmd

		stdin  io.WriteCloser
		stdout *bufio.Reader
	}

	// ch holds the output queues consumed by Read().
	ch struct {
		stdoutEvents chan readResult // meaningful stdout events from the readStdout goroutine
		stderrLines  chan string     // diagnostic chunks from the readStderr goroutine
	}

	// rpc serializes requests written to stdin and assigns request IDs.
	rpc struct {
		mu    sync.Mutex
		msgID int
		obs   ProtocolObserver
	}

	// turn tracks the in-flight turn lifecycle within the current thread.
	turn struct {
		mu       sync.Mutex
		threadID string
		state    turnState
	}
}

type turnStateKind uint8

const (
	turnIdle turnStateKind = iota
	turnInflightUnknownID
	turnInflightKnownID
)

type turnState struct {
	kind   turnStateKind
	turnID string
}

// Option configures a Client at construction time.
type Option func(*Client)

// WithObserver attaches a ProtocolObserver that receives every raw JSON line.
func WithObserver(obs ProtocolObserver) Option {
	return func(c *Client) { c.rpc.obs = obs }
}

// New returns a Client that will run Codex in the given working directory.
func New(cwd string, opts ...Option) *Client {
	c := &Client{}
	c.proc.cwd = cwd
	c.rpc.obs = noopObserver{}
	c.ch.stdoutEvents = make(chan readResult)
	c.ch.stderrLines = make(chan string)
	for _, o := range opts {
		o(c)
	}
	return c
}

// Start launches the Codex app-server and completes the initialize and
// thread/start handshakes. It must be called before Send or Read.
func (c *Client) Start() error {
	args := codexArgs()
	cmd := exec.Command(args[0], args[1:]...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("codex stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("codex stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("codex stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("codex start: %w", err)
	}

	c.proc.cmd = cmd
	c.proc.stdin = stdin
	c.proc.stdout = bufio.NewReader(stdout)
	go c.readStderr(stderr)

	if err := c.initialize(); err != nil {
		_ = c.Stop()
		return err
	}
	threadID, err := c.startThread()
	if err != nil {
		_ = c.Stop()
		return err
	}
	c.turn.mu.Lock()
	c.turn.threadID = threadID
	c.turn.state = turnState{kind: turnIdle}
	c.turn.mu.Unlock()
	go c.readStdout()
	return nil
}

// Interrupt requests the Codex process to stop its current in-flight work.
// If a turn is active, send a turn/interrupt request.
func (c *Client) Interrupt() error {
	c.turn.mu.Lock()
	threadID := c.turn.threadID
	state := c.turn.state
	c.turn.mu.Unlock()

	// if the turn is being established, there is no active work being done
	// and we don't know the turn ID yet, so we can't send turn/interrupt.
	if state.kind == turnInflightUnknownID {
		return nil
	}
	if threadID == "" || state.turnID == "" {
		return nil
	}
	return c.writeRequest("turn/interrupt", map[string]any{
		"threadId": threadID,
		"turnId":   state.turnID,
	})
}

// Send writes a turn/start request to stdin and returns immediately.
// It does not read from stdout. Notifications arrive via Read().
func (c *Client) Send(prompt string) error {
	c.turn.mu.Lock()
	if c.turn.state.kind != turnIdle {
		c.turn.mu.Unlock()
		return agent.ErrTurnInProgress
	}
	c.turn.state = turnState{kind: turnInflightUnknownID}
	threadID := c.turn.threadID
	c.turn.mu.Unlock()

	err := c.writeRequest("turn/start", map[string]any{
		"threadId": threadID,
		"input":    []map[string]any{{"type": "text", "text": prompt}},
	})
	if err != nil {
		c.turn.mu.Lock()
		c.turn.state = turnState{kind: turnIdle}
		c.turn.mu.Unlock()
	}
	return err
}

// Read blocks until a meaningful event is ready — either a stdout-derived
// notification (Delta, Done) or a queued stderr line (Log). Both sources are
// waited on simultaneously so neither can stall the other. A closed
// stdoutEvents channel means the process has exited and no further events
// will arrive.
func (c *Client) Read() (agent.Event, error) {
	select {
	case r, ok := <-c.ch.stdoutEvents:
		if !ok {
			return agent.Event{}, fmt.Errorf("codex: process exited")
		}
		return r.ev, r.err
	case line := <-c.ch.stderrLines:
		return agent.Event{Log: line}, nil
	}
}

// readStdout bridges the stdout pipe to stdoutEvents via an unbounded internal
// buffer. Memory grows if the consumer falls behind; acceptable for a local
// CLI tool where stdout throughput is bounded by the agent's output rate. stdoutEvents is closed when the pump exits so Read() can detect
// process exit. The drain on pipe-close is best-effort (non-blocking send):
// if the consumer is gone the remaining items are dropped, but the closed
// channel still signals Read() that no further events will arrive.
func (c *Client) readStdout() {
	defer close(c.ch.stdoutEvents)
	in := make(chan readResult)
	go c.scanStdout(in)
	var buf []readResult
	for {
		if len(buf) == 0 {
			r, ok := <-in
			if !ok {
				return
			}
			buf = append(buf, r)
		} else {
			select {
			case r, ok := <-in:
				if !ok {
					for _, pending := range buf {
						select {
						case c.ch.stdoutEvents <- pending:
						default:
						}
					}
					return
				}
				buf = append(buf, r)
			case c.ch.stdoutEvents <- buf[0]:
				buf = buf[1:]
			}
		}
	}
}

// scanStdout reads raw stdout lines, translates them to readResult values, and
// sends meaningful ones on in. It closes in when the pipe closes or a
// terminal error (turn/failed, IO error) is encountered.
func (c *Client) scanStdout(in chan<- readResult) {
	for {
		raw, err := c.proc.stdout.ReadString('\n')
		if err != nil {
			in <- readResult{err: fmt.Errorf("codex stdout: %w", err)}
			close(in)
			return
		}
		raw = strings.TrimRight(raw, "\r\n")
		c.rpc.obs.OnReceive(raw)
		var msg rpcMsg
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			continue
		}
		if msg.Method == "" {
			continue
		}
		c.noteNotification(msg)
		ev, ok, err := translateNotification(msg)
		if err != nil {
			in <- readResult{err: err}
			close(in)
			return
		}
		if ok {
			in <- readResult{ev: ev}
		}
	}
}

func (c *Client) noteNotification(msg rpcMsg) {
	switch msg.Method {
	case "turn/started":
		var p struct {
			ThreadID string `json:"threadId"`
			Turn     struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return
		}
		if p.ThreadID == "" || p.Turn.ID == "" {
			return
		}
		c.turn.mu.Lock()
		// Ignore stray turns from other threads.
		if c.turn.threadID == p.ThreadID {
			c.turn.state = turnState{kind: turnInflightKnownID, turnID: p.Turn.ID}
		}
		c.turn.mu.Unlock()
	case "turn/completed", "turn/failed":
		c.turn.mu.Lock()
		c.turn.state = turnState{kind: turnIdle}
		c.turn.mu.Unlock()
	}
}

// translateNotification maps a known Codex notification to an agent.Event.
// Returns ok=false for unknown notifications (caller should discard and continue).
func translateNotification(msg rpcMsg) (agent.Event, bool, error) {
	switch msg.Method {
	case "item/agentMessage/delta":
		var p struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return agent.Event{}, false, fmt.Errorf("parse delta params: %w", err)
		}
		return agent.Event{Delta: p.Delta}, true, nil
	case "turn/completed":
		return agent.Event{Done: true}, true, nil
	case "turn/failed":
		return agent.Event{}, false, fmt.Errorf("turn failed: %s", msg.Params)
	}
	return agent.Event{}, false, nil
}

const stopGracePeriod = 5 * time.Second

// Stop closes stdin and waits for the Codex process to exit.
// If the process does not exit within stopGracePeriod it is killed.
// May be called from a different goroutine to interrupt a blocked Read().
func (c *Client) Stop() error {
	if c.proc.stdin != nil {
		_ = c.proc.stdin.Close()
	}
	if c.proc.cmd == nil || c.proc.cmd.Process == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- c.proc.cmd.Wait() }()

	timer := time.NewTimer(stopGracePeriod)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("codex wait: %w", err)
		}
		return nil
	case <-timer.C:
		_ = c.proc.cmd.Process.Kill()
		<-done
		return nil
	}
}

func (c *Client) initialize() error {
	if err := c.writeRequest("initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "coderoom", "version": "0.1.0"},
		"capabilities": map[string]any{"experimentalApi": true},
	}); err != nil {
		return err
	}
	_, err := c.readResponse()
	return err
}

func (c *Client) startThread() (string, error) {
	if err := c.writeRequest("thread/start", map[string]any{"cwd": c.proc.cwd}); err != nil {
		return "", err
	}
	raw, err := c.readResponse()
	if err != nil {
		return "", err
	}
	var r struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("parse thread result: %w", err)
	}
	return r.Thread.ID, nil
}

// writeRequest serialises a JSON-RPC request, notifies the observer, and
// writes it to stdin.
func (c *Client) writeRequest(method string, params any) error {
	c.rpc.mu.Lock()
	defer c.rpc.mu.Unlock()

	c.rpc.msgID++
	b, err := json.Marshal(map[string]any{
		"method": method,
		"id":     c.rpc.msgID,
		"params": params,
	})
	if err != nil {
		return fmt.Errorf("marshal rpc: %w", err)
	}
	c.rpc.obs.OnSend(string(b))
	if _, err := fmt.Fprintf(c.proc.stdin, "%s\n", b); err != nil {
		return fmt.Errorf("write rpc: %w", err)
	}
	return nil
}

// readResponse reads lines until it finds an RPC response (ID-bearing).
// Notification lines encountered during the handshake are logged and discarded.
func (c *Client) readResponse() (json.RawMessage, error) {
	for {
		raw, err := c.proc.stdout.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("codex stdout: %w", err)
		}
		raw = strings.TrimRight(raw, "\r\n")
		c.rpc.obs.OnReceive(raw)
		var msg rpcMsg
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			continue
		}
		if msg.ID == nil {
			// Notification during handshake — unexpected but non-fatal; discard.
			continue
		}
		if *msg.ID != c.rpc.msgID {
			return nil, fmt.Errorf("codex: unexpected response id %d (expected %d)", *msg.ID, c.rpc.msgID)
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("rpc error: %s", msg.Error)
		}
		return msg.Result, nil
	}
}

// readStderr bridges the process stderr pipe to stderrLines via an unbounded
// internal buffer (same trade-off as readStdout: memory grows if the consumer
// falls behind, acceptable for a local CLI). When the pipe
// closes (process exited), remaining buffered lines are discarded rather than
// blocking on stderrLines — diagnostic output at shutdown is not worth a
// goroutine leak.
func (c *Client) readStderr(r io.Reader) {
	for chunk := range linestream.BatchReader(r) {
		c.ch.stderrLines <- chunk
	}
}

// codexArgs returns the command and arguments for the Codex app-server.
// CODEX_VERSION_OVERRIDE pins a specific npm version for integration testing.
func codexArgs() []string {
	pkg := "@openai/codex"
	if v := os.Getenv("CODEX_VERSION_OVERRIDE"); v != "" {
		pkg = "@openai/codex@" + v
	}
	return []string{"npx", pkg, "app-server"}
}
