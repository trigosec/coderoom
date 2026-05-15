package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type nonJSONStdoutLineError struct {
	line string
}

func (e nonJSONStdoutLineError) Error() string {
	return "parse stdout json: invalid json"
}

func (e nonJSONStdoutLineError) Line() string { return e.line }

func (e nonJSONStdoutLineError) FormatLogLine() string {
	const maxLen = 200
	line := e.line
	if line == "" {
		return "codex stdout: <empty non-json line>"
	}
	if len(line) > maxLen {
		line = line[:maxLen] + "…"
	}
	return "codex stdout (non-json): " + line
}

// rpcWrite writes a JSON-RPC request to the Codex process stdin.
//
// It is blocking and serialized via c.rpc.mu to ensure request IDs are
// monotonic and writes are not interleaved.
func rpcWrite[T any](c *Client, method string, params T) error {
	c.rpc.mu.Lock()
	defer c.rpc.mu.Unlock()

	c.rpc.msgID++
	b, err := json.Marshal(rpcRequest[T]{
		Method: method,
		ID:     c.rpc.msgID,
		Params: params,
	})
	if err != nil {
		return fmt.Errorf("marshal rpc: %w", err)
	}
	c.rpc.obs.OnSend(string(b))
	if _, err := fmt.Fprintf(c.proc.codexIn, "%s\n", b); err != nil {
		return fmt.Errorf("write rpc: %w", err)
	}
	return nil
}

// rpcHandshake performs the initialize and thread/start handshake sequence and
// returns the active thread ID.
func rpcHandshake(c *Client) (string, error) {
	var init initializeParams
	init.ClientInfo.Name = "coderoom"
	init.ClientInfo.Version = "0.1.0"
	init.Capabilities.ExperimentalAPI = true
	if err := rpcWrite(c, methodInitialize, init); err != nil {
		return "", err
	}
	if _, err := readResponse(c); err != nil {
		return "", err
	}

	if err := rpcWrite(c, methodThreadStart, threadStartParams{Cwd: c.proc.cwd}); err != nil {
		return "", err
	}
	raw, err := readResponse(c)
	if err != nil {
		return "", err
	}
	var r threadStartResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("parse thread result: %w", err)
	}
	return r.Thread.ID, nil
}

// readResponse reads lines until it finds an RPC response (ID-bearing).
// Notification lines encountered during the handshake are logged and discarded.
func readResponse(c *Client) (json.RawMessage, error) {
	for {
		msg, err := rpcRead(c)
		if err != nil {
			if _, ok := isNonJSONStdoutLine(err); ok {
				// Ignore non-JSON noise during handshake.
				continue
			}
			return nil, err
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

// rpcRead reads one newline-delimited JSON message from the Codex process
// stdout and decodes it into an rpcEnvelope.
func rpcRead(c *Client) (rpcEnvelope, error) {
	raw, err := c.proc.codexOut.ReadString('\n')
	if err != nil {
		return rpcEnvelope{}, fmt.Errorf("codex stdout: %w", err)
	}
	raw = strings.TrimRight(raw, "\r\n")
	c.rpc.obs.OnReceive(raw)

	var env rpcEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		// Defensive: some CLIs can emit banners/progress lines on stdout. Treat
		// these as non-fatal so the session stays usable.
		return rpcEnvelope{}, nonJSONStdoutLineError{line: raw}
	}
	return env, nil
}

func isNonJSONStdoutLine(err error) (nonJSONStdoutLineError, bool) {
	var e nonJSONStdoutLineError
	if errors.As(err, &e) {
		return e, true
	}
	return nonJSONStdoutLineError{}, false
}
