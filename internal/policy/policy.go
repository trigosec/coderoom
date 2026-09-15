// Package policy enforces session-level constraints: file write permissions,
// initiative behaviour, and risk boundaries per participant.
package policy

import (
	"fmt"
	"sync"
)

// Name identifies a room-local runtime policy.
type Name string

const (
	// SendNotices enables listener notices for direct shared-room sends.
	SendNotices Name = "send-notices"
	// EchoInvites makes subsequent invitations use the deterministic echo adapter.
	EchoInvites Name = "echo-invites"
)

// Set provides concurrency-safe access to enabled room policies. Copies refer
// to the same underlying policy set. Construct a Set with NewSet.
type Set struct {
	state *state
}

// state is held indirectly so copying Set never copies a mutex or forks
// the enabled-policy map.
type state struct {
	mu      sync.RWMutex
	enabled map[Name]struct{}
}

// NewSet constructs an empty room-local policy set.
func NewSet() Set {
	return Set{state: &state{enabled: make(map[Name]struct{})}}
}

// Enable turns on name. Enabling an already-enabled policy is a no-op.
func (p Set) Enable(name Name) error {
	if name != SendNotices && name != EchoInvites {
		return fmt.Errorf("unknown policy %q", name)
	}
	if p.state == nil {
		return fmt.Errorf("policies are not initialized")
	}
	p.state.mu.Lock()
	defer p.state.mu.Unlock()
	p.state.enabled[name] = struct{}{}
	return nil
}

// Enabled reports whether name is enabled.
func (p Set) Enabled(name Name) bool {
	if p.state == nil {
		return false
	}
	p.state.mu.RLock()
	defer p.state.mu.RUnlock()
	_, ok := p.state.enabled[name]
	return ok
}
