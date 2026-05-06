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

// Client implements agent.Agent for the Codex app-server.
// Calls must not be made concurrently, except Stop() which may be called
// from another goroutine to interrupt a blocked Read().
type Client struct {
	cwd      string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	reader   *bufio.Reader
	msgID    int
	threadID string
	obs      ProtocolObserver
}

// Option configures a Client at construction time.
type Option func(*Client)

// WithObserver attaches a ProtocolObserver that receives every raw JSON line.
func WithObserver(obs ProtocolObserver) Option {
	return func(c *Client) { c.obs = obs }
}

// New returns a Client that will run Codex in the given working directory.
func New(cwd string, opts ...Option) *Client {
	c := &Client{cwd: cwd, obs: noopObserver{}}
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
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("codex stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("codex stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("codex start: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.reader = bufio.NewReader(stdout)

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

// Read blocks until a meaningful event is ready, translating Codex notifications
// into agent.Event. Unknown notifications are discarded; the observer records them.
// Returns an error if the process has exited or the turn has failed.
func (c *Client) Read() (agent.Event, error) {
	for {
		raw, err := c.reader.ReadString('\n')
		if err != nil {
			return agent.Event{}, fmt.Errorf("codex stdout: %w", err)
		}
		raw = strings.TrimRight(raw, "\r\n")
		c.obs.OnReceive(raw)
		var msg rpcMsg
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			continue
		}
		if msg.Method == "" {
			// Response line (ID-bearing, no method): unexpected after Start();
			// skip deliberately — observer has recorded it for diagnosis.
			continue
		}
		switch msg.Method {
		case "item/agentMessage/delta":
			var p struct {
				Delta string `json:"delta"`
			}
			if err := json.Unmarshal(msg.Params, &p); err != nil {
				continue
			}
			return agent.Event{Delta: p.Delta}, nil
		case "turn/completed":
			return agent.Event{Done: true}, nil
		case "turn/failed":
			return agent.Event{}, fmt.Errorf("turn failed: %s", msg.Params)
		}
	}
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

// codexArgs returns the command and arguments for the Codex app-server.
// CODEX_VERSION_OVERRIDE pins a specific npm version for integration testing.
func codexArgs() []string {
	pkg := "@openai/codex"
	if v := os.Getenv("CODEX_VERSION_OVERRIDE"); v != "" {
		pkg = "@openai/codex@" + v
	}
	return []string{"npx", pkg, "app-server"}
}
