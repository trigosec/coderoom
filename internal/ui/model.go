// Package ui implements the terminal interface using Bubble Tea.
// model.go defines the application state.
package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/session"
)

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
	input           textinput.Model
	records         []record
	renderedRecords []string        // rendered form of each record; rebuilt on resize
	streaming       map[string]int  // alias → index in records (agents mid-turn)
	departed        map[string]bool // aliases that have left; kept for grey repaint on resize
	palette         colorPalette
	cwd             string
	ready           bool // true after first WindowSizeMsg
}

// New creates a Model with its own session and event queue.
func New(cwd string) Model {
	q := newEventQueue()
	sess := session.New(session.WithObserver(channelObserver{queue: q}))

	ti := textinput.New()
	ti.Prompt = "> "
	ti.Focus()

	return Model{
		sess:            sess,
		queue:           q,
		input:           ti,
		records:         []record{},
		renderedRecords: []string{},
		streaming:       make(map[string]int),
		departed:        make(map[string]bool),
		cwd:             cwd,
	}
}
