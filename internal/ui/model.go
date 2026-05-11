// Package ui implements the terminal interface using Bubble Tea.
// model.go defines the application state.
package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/session"
)

// Option configures a Model at construction time.
type Option func(*Model)

// WithAgentFactory sets the factory used to construct agents when a participant
// is invited. The factory receives the agent alias and working directory.
func WithAgentFactory(f func(alias, cwd string) agent.Agent) Option {
	return func(m *Model) { m.agentFactory = f }
}

// WithDebug enables developer debugging features (debug commands and optional
// overlays). Intended to be wired to CODEROOM_DEBUG=1 in the CLI.
func WithDebug(enabled bool) Option {
	return func(m *Model) { m.debug = enabled }
}

// sessionEventMsg wraps a session.Event as a Bubble Tea message.
type sessionEventMsg session.Event

// awaitEvent returns a Cmd that blocks until the next event is available.
func awaitEvent(q *eventQueue) tea.Cmd {
	return func() tea.Msg {
		e, ok := q.Pull()
		if !ok {
			return nil
		}
		return sessionEventMsg(e)
	}
}

// channelObserver implements session.Observer by pushing events into an
// eventQueue. It is safe to call from any goroutine.
type channelObserver struct {
	queue *eventQueue
}

func (o channelObserver) OnEvent(e session.Event) {
	o.queue.Push(e)
}

// Model is the Bubble Tea application state for the coderoom TUI.
type Model struct {
	sess            *session.Session
	queue           *eventQueue
	viewport        viewport.Model
	input           textarea.Model
	focus           focusTarget
	debug           bool
	debugRowNums    bool
	tickActive      bool
	records         []record
	renderedRecords []string        // rendered form of each record; rebuilt on resize
	streaming       map[string]int  // alias → index in records (agents mid-turn)
	departed        map[string]bool // aliases that have left; kept for grey repaint on resize
	agentFactory    func(alias, cwd string) agent.Agent
	palette         colorPalette
	cwd             string
	ready           bool // true after first WindowSizeMsg
	lastSize        tea.WindowSizeMsg
	now             func() time.Time
}

type focusTarget int

const (
	focusComposer focusTarget = iota
	focusViewport
)

// New creates a Model with its own session and event queue.
func New(cwd string, opts ...Option) Model {
	q := newEventQueue()
	sess := session.New(session.WithObserver(channelObserver{queue: q}))

	ti := textarea.New()
	ti.ShowLineNumbers = false
	ti = updateInputDecorations(ti)
	ti.Focus()

	m := Model{
		sess:            sess,
		queue:           q,
		input:           ti,
		focus:           focusComposer,
		records:         []record{},
		renderedRecords: []string{},
		streaming:       make(map[string]int),
		departed:        make(map[string]bool),
		cwd:             cwd,
		now:             time.Now,
	}
	for _, o := range opts {
		o(&m)
	}
	return m
}
