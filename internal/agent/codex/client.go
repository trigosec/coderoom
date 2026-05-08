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
	"time"

	"github.com/trigosec/coderoom/internal/agent"
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
	cwd          string
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	reader       *bufio.Reader
	stdoutEvents chan readResult // meaningful stdout events from the readStdout goroutine
	stderrLines  chan string     // diagnostic lines from the readStderr goroutine
	msgID        int
	threadID     string
	obs          ProtocolObserver
}

// Option configures a Client at construction time.
type Option func(*Client)

// WithObserver attaches a ProtocolObserver that receives every raw JSON line.
func WithObserver(obs ProtocolObserver) Option {
	return func(c *Client) { c.obs = obs }
}

// New returns a Client that will run Codex in the given working directory.
func New(cwd string, opts ...Option) *Client {
	c := &Client{
		cwd:          cwd,
		obs:          noopObserver{},
		stdoutEvents: make(chan readResult),
		stderrLines:  make(chan string),
	}
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

	c.cmd = cmd
	c.stdin = stdin
	c.reader = bufio.NewReader(stdout)
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
	c.threadID = threadID
	go c.readStdout()
	return nil
}

// Send writes a turn/start request to stdin and returns immediately.
// It does not read from stdout. Notifications arrive via Read().
func (c *Client) Send(prompt string) error {
	return c.writeRequest("turn/start", map[string]any{
		"threadId": c.threadID,
		"input":    []map[string]any{{"type": "text", "text": prompt}},
	})
}

// Read blocks until a meaningful event is ready — either a stdout-derived
// notification (Delta, Done) or a queued stderr line (Log). Both sources are
// waited on simultaneously so neither can stall the other. A closed
// stdoutEvents channel means the process has exited and no further events
// will arrive.
func (c *Client) Read() (agent.Event, error) {
	select {
	case r, ok := <-c.stdoutEvents:
		if !ok {
			return agent.Event{}, fmt.Errorf("codex: process exited")
		}
		return r.ev, r.err
	case line := <-c.stderrLines:
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
	defer close(c.stdoutEvents)
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
						case c.stdoutEvents <- pending:
						default:
						}
					}
					return
				}
				buf = append(buf, r)
			case c.stdoutEvents <- buf[0]:
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
		raw, err := c.reader.ReadString('\n')
		if err != nil {
			in <- readResult{err: fmt.Errorf("codex stdout: %w", err)}
			close(in)
			return
		}
		raw = strings.TrimRight(raw, "\r\n")
		c.obs.OnReceive(raw)
		var msg rpcMsg
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			continue
		}
		if msg.Method == "" {
			continue
		}
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
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()

	timer := time.NewTimer(stopGracePeriod)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("codex wait: %w", err)
		}
		return nil
	case <-timer.C:
		_ = c.cmd.Process.Kill()
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
	if err := c.writeRequest("thread/start", map[string]any{"cwd": c.cwd}); err != nil {
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
	c.msgID++
	b, err := json.Marshal(map[string]any{
		"method": method,
		"id":     c.msgID,
		"params": params,
	})
	if err != nil {
		return fmt.Errorf("marshal rpc: %w", err)
	}
	c.obs.OnSend(string(b))
	if _, err := fmt.Fprintf(c.stdin, "%s\n", b); err != nil {
		return fmt.Errorf("write rpc: %w", err)
	}
	return nil
}

// readResponse reads lines until it finds an RPC response (ID-bearing).
// Notification lines encountered during the handshake are logged and discarded.
func (c *Client) readResponse() (json.RawMessage, error) {
	for {
		raw, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("codex stdout: %w", err)
		}
		raw = strings.TrimRight(raw, "\r\n")
		c.obs.OnReceive(raw)
		var msg rpcMsg
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			continue
		}
		if msg.ID == nil {
			// Notification during handshake — unexpected but non-fatal; discard.
			continue
		}
		if *msg.ID != c.msgID {
			return nil, fmt.Errorf("codex: unexpected response id %d (expected %d)", *msg.ID, c.msgID)
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
	in := make(chan string)
	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			in <- scanner.Text()
		}
		close(in)
	}()
	var buf []string
	for {
		if len(buf) == 0 {
			line, ok := <-in
			if !ok {
				return
			}
			buf = append(buf, line)
		} else {
			select {
			case line, ok := <-in:
				if !ok {
					return // process exited; discard remaining buf to avoid goroutine leak
				}
				buf = append(buf, line)
			case c.stderrLines <- buf[0]:
				buf = buf[1:]
			}
		}
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
