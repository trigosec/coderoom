package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/session"
	"github.com/trigosec/coderoom/internal/ui/room/history"
)

// makeReadyModel returns a Model that has processed one WindowSizeMsg so the
// viewport is initialised and syncViewport calls are live.
func makeReadyModel(t *testing.T) Model {
	t.Helper()
	m := New(newTestSession(), ".")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(Model)
}

func makeReadyModelWithHeight(t *testing.T, height int) Model {
	t.Helper()
	m := New(newTestSession(), ".")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: height})
	return next.(Model)
}

// newTestSession returns a bare session suitable for UI unit tests (no factory).
func newTestSession() *session.Session {
	return session.New()
}

// pushEvent sends a session event into the model via Update and returns the result.
func pushEvent(m Model, e session.Event) Model {
	next, _ := m.Update(sessionEventMsg(e))
	return next.(Model)
}

// hasRecord reports whether any record of the given kind contains text in its body.
func hasRecord(m Model, kind history.RecordKind, text string) bool {
	for _, r := range m.room.HistoryRecords() {
		if r.Kind != kind {
			continue
		}
		body := r.Text
		if r.Msg != nil {
			switch c := r.Msg.Content.(type) {
			case agent.Output:
				body = c.Text
			case agent.Reasoning:
				body = c.Text
			case agent.Command:
				body = c.Output
			case agent.FileChangeSet:
				body = history.FormatFileChangeBody(c.Changes)
			case agent.Log:
				body = c.Text
			}
		}
		if strings.Contains(body, text) {
			return true
		}
	}
	return false
}
