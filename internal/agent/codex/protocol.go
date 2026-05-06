package codex

import "encoding/json"

// ProtocolObserver receives every raw JSON line as it flows over stdio,
// before any parsing. Implementations must be fast; avoid operations that
// can block for non-trivial time (network calls, contested locks). A log
// file write is acceptable. msg is the raw JSON without the trailing newline.
type ProtocolObserver interface {
	OnSend(msg string)
	OnReceive(msg string)
}

// noopObserver is the default ProtocolObserver when none is provided.
type noopObserver struct{}

func (noopObserver) OnSend(string)    {}
func (noopObserver) OnReceive(string) {}

// rpcMsg covers requests, responses, and notifications.
// Requests and notifications have Method set.
// Responses have ID set with Result or Error.
type rpcMsg struct {
	Method string          `json:"method,omitempty"`
	ID     *int            `json:"id,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}
