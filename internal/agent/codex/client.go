// Package codex implements the Agent interface for the Codex CLI app-server.
// Communication uses JSON-RPC 2.0 over stdio (newline-delimited JSON).
// See docs/design/pkg-agent-codex.md for the full design rationale.
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
)

// readEvent carries the outcome of a single stdout notification.
type readEvent struct {
	ev  agent.Event
	err error
}

// Client implements agent.Agent for the Codex app-server.
// Calls must not be made concurrently, except Stop() which may be called
// from another goroutine to interrupt a blocked Read().
type Client struct {
	// proc holds the OS process and stdio pipes.
	proc *process

	// ch holds the output queues consumed by Read().
	read struct {
		events chan readEvent // events from readStdout and readCodexErrWorker
	}

	// rpc serializes requests written to codex stdin and assigns request IDs.
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

	lifecycle struct {
		ctx      context.Context
		cancelFn context.CancelFunc
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
	c := &Client{proc: newProc(cwd)}
	c.rpc.obs = noopObserver{}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) initRead() {
	if c.read.events == nil {
		c.read.events = make(chan readEvent)
	}
}

// Start launches the Codex app-server and completes the initialize and
// thread/start handshakes. It must be called before Send or Read.
func (c *Client) Start() error {
	err := c.proc.start()
	if err != nil {
		return err
	}

	c.initRead()

	threadID, err := rpcHandshake(c)
	if err != nil {
		close(c.read.events)
		c.read.events = nil
		_ = c.Stop()
		return err
	}
	c.turn.mu.Lock()
	c.turn.threadID = threadID
	c.turn.state = turnState{kind: turnIdle}
	c.turn.mu.Unlock()

	if c.lifecycle.ctx == nil {
		c.lifecycle.ctx, c.lifecycle.cancelFn = context.WithCancel(context.Background()) // #nosec: G118
	}
	var workers = []workerFn{readCodexErrWorker}
	var wg sync.WaitGroup
	wg.Add(len(workers) + 1)
	// readStdout is for the moment a special case, until I complete the refactoring
	// I am using readCodexErrWorker as the initial poc
	go func() {
		defer wg.Done()
		c.readStdout()
	}()
	for _, worker := range workers {
		worker := worker
		go func() {
			defer wg.Done()
			worker(c.lifecycle.ctx, c)
		}()
	}
	go func() {
		wg.Wait()
		close(c.read.events)
	}()
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
	return rpcWrite(c, methodTurnInterrupt, turnInterruptParams{
		ThreadID: threadID,
		TurnID:   state.turnID,
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

	err := rpcWrite(c, methodTurnStart, turnStartParams{
		ThreadID: threadID,
		Input:    []turnInput{{Type: "text", Text: prompt}},
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
// read.events channel means the process has exited and no further events
// will arrive.
func (c *Client) Read() (agent.Event, error) {
	if c.read.events == nil {
		return agent.Event{}, fmt.Errorf("codex: client not started")
	}
	r, ok := <-c.read.events
	if !ok {
		return agent.Event{}, fmt.Errorf("codex: process exited")
	}
	return r.ev, r.err
}

// readStdout bridges the stdout pipe to read.events via an unbounded internal
// buffer. Memory grows if the consumer falls behind; acceptable for a local
// CLI tool where stdout throughput is bounded by the agent's output rate.
//
// readStdout does not close read.events directly because stderr also writes to
// it. Start() closes read.events after both workers exit.
func (c *Client) readStdout() {
	in := make(chan readEvent)
	go c.scanStdout(in)
	var buf []readEvent
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
						case c.read.events <- pending:
						default:
						}
					}
					return
				}
				buf = append(buf, r)
			case c.read.events <- buf[0]:
				buf = buf[1:]
			}
		}
	}
}

// scanStdout reads raw stdout lines, translates them to readEvent values, and
// sends meaningful ones on in. It closes in when the pipe closes or a
// terminal error (turn/failed, IO error) is encountered.
func (c *Client) scanStdout(in chan<- readEvent) {
	for {
		msg, err := rpcRead(c)
		if err != nil {
			if nonJSON, ok := isNonJSONStdoutLine(err); ok {
				in <- readEvent{ev: agent.Event{Log: nonJSON.FormatLogLine()}}
				continue
			}
			in <- readEvent{err: err}
			close(in)
			return
		}
		if msg.Method == "" {
			continue
		}
		c.noteNotification(msg)
		ev, ok, err := translateNotification(msg)
		if err != nil {
			in <- readEvent{err: err}
			close(in)
			return
		}
		if ok {
			in <- readEvent{ev: ev}
		}
	}
}

func (c *Client) noteNotification(msg rpcEnvelope) {
	switch msg.Method {
	case methodTurnStarted:
		var p turnStartedParams
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
	case methodTurnCompleted, methodTurnFailed:
		c.turn.mu.Lock()
		c.turn.state = turnState{kind: turnIdle}
		c.turn.mu.Unlock()
	}
}

// translateNotification maps a known Codex notification to an agent.Event.
// Returns ok=false for unknown notifications (caller should discard and continue).
func translateNotification(msg rpcEnvelope) (agent.Event, bool, error) {
	switch msg.Method {
	case methodAgentDelta:
		var p deltaParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return agent.Event{}, false, fmt.Errorf("parse delta params: %w", err)
		}
		return agent.Event{Delta: p.Delta}, true, nil
	case methodTurnCompleted:
		return agent.Event{Done: true}, true, nil
	case methodTurnFailed:
		return agent.Event{}, false, fmt.Errorf("turn failed: %s", msg.Params)
	}
	return agent.Event{}, false, nil
}

const stopGracePeriod = 5 * time.Second

// Stop closes stdin and waits for the Codex process to exit.
// If the process does not exit within stopGracePeriod it is killed.
// May be called from a different goroutine to interrupt a blocked Read().
func (c *Client) Stop() error {
	if c.lifecycle.cancelFn != nil {
		c.lifecycle.cancelFn()
		c.lifecycle.cancelFn = nil
	}
	if c.proc.codexIn != nil {
		_ = c.proc.codexIn.Close()
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

// readResponse reads lines until it finds an RPC response (ID-bearing).
// Notification lines encountered during the handshake are logged and discarded.
func (c *Client) readResponse() (json.RawMessage, error) {
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
