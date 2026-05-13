package codex

import "encoding/json"

// rpcRequest is a minimal JSON-RPC request envelope (newline-delimited JSON).
// Codex does not require the jsonrpc version field in practice.
type rpcRequest struct {
	Method string `json:"method"`
	ID     int    `json:"id"`
	Params any    `json:"params,omitempty"`
}

type initializeParams struct {
	ClientInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
	Capabilities struct {
		ExperimentalAPI bool `json:"experimentalApi"`
	} `json:"capabilities"`
}

type threadStartParams struct {
	Cwd string `json:"cwd"`
}

type threadStartResult struct {
	Thread struct {
		ID string `json:"id"`
	} `json:"thread"`
}

type turnStartParams struct {
	ThreadID string      `json:"threadId"`
	Input    []turnInput `json:"input"`
}

type turnInput struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type turnInterruptParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}

type turnStartedParams struct {
	ThreadID string `json:"threadId"`
	Turn     struct {
		ID string `json:"id"`
	} `json:"turn"`
}

type deltaParams struct {
	Delta string `json:"delta"`
}

// rpcEnvelope is the minimal wire envelope used for decoding unknown messages.
// Params/Result are left as RawMessage so they can be routed by Method first.
type rpcEnvelope struct {
	Method string          `json:"method,omitempty"`
	ID     *int            `json:"id,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}
