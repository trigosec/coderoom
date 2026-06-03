package transcript

import (
	"encoding/json"
	"sync"
)

// Observer captures raw JSON-RPC traffic as replayable transcript steps.
// It satisfies codex.ProtocolObserver by method set, without coupling this
// package to internal/agent/codex.
type Observer struct {
	mu    sync.Mutex
	steps []Step
}

// NewObserver returns a fresh protocol observer that records transcript steps.
func NewObserver() *Observer {
	return &Observer{}
}

// OnSend records one outbound JSON-RPC line as a replay "recv" step.
func (o *Observer) OnSend(msg string) {
	o.append("recv", msg)
}

// OnReceive records one inbound JSON-RPC line as a replay "send" step.
func (o *Observer) OnReceive(msg string) {
	o.append("send", msg)
}

// Steps returns a stable snapshot of the recorded transcript steps.
func (o *Observer) Steps() []Step {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]Step, len(o.steps))
	copy(out, o.steps)
	return out
}

func (o *Observer) append(kind string, raw string) {
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return
	}
	step := Step{Kind: kind}
	if kind == "recv" {
		step.Match = payload
	} else {
		step.Message = payload
	}
	o.mu.Lock()
	o.steps = append(o.steps, step)
	o.mu.Unlock()
}
