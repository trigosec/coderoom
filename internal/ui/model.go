// Package ui implements the terminal interface using Bubble Tea.
// model.go defines the application state.
package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/session"
	"github.com/trigosec/coderoom/internal/ui/room/compose"
	"github.com/trigosec/coderoom/internal/ui/room/history"
	"github.com/trigosec/coderoom/internal/ui/toolbox"
)

// Option configures a Model at construction time.
type Option func(*Model)

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
	sess     *session.Session
	queue    *eventQueue
	history  history.Model
	compose  compose.Model
	toolbox  toolbox.Model
	focus    focusTarget
	debug    bool
	palette  colorPalette
	cwd      string
	lastSize tea.WindowSizeMsg
}

type focusTarget int

const (
	focusComposer focusTarget = iota
	focusViewport
)

// New creates a Model backed by the given session.
// The session must have an AgentFactory configured before any invite commands
// are executed.
func New(sess *session.Session, cwd string, opts ...Option) Model {
	q := newEventQueue()
	sess.AddObserver(channelObserver{queue: q})

	colorByAlias := func(alias string) string {
		if p, ok := sess.Participant(alias); ok {
			return p.Color
		}
		return ""
	}

	m := Model{
		sess:    sess,
		queue:   q,
		history: history.New(colorByAlias, ColorDeparted),
		compose: compose.New(),
		toolbox: toolbox.New(),
		focus:   focusComposer,
		cwd:     cwd,
	}
	for _, o := range opts {
		o(&m)
	}
	return m
}
