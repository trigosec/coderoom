package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/trigosec/coderoom/internal/session"
	"github.com/trigosec/coderoom/internal/ui/room/history/record"
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
	m.room.SessionObserver().OnEvent(e)
	next, _ := m.Update(sessionEventMsg{event: e})
	out := next.(Model)
	if roomEventProducesUpdate(e) {
		if updated, ok := out.room.WaitObserverUpdateTimeout(2 * time.Second); ok {
			out.room = updated
		}
	}
	return out
}

func roomEventProducesUpdate(e session.Event) bool {
	switch e.(type) {
	case session.AgentStarting,
		session.AgentStarted,
		session.AgentStopped,
		session.AgentCrashed,
		session.AgentLog,
		session.ContextHandoff,
		session.AgentMessage:
		return true
	default:
		return false
	}
}

// hasRecord reports whether any record of the given kind contains text in its body.
func hasRecord(m Model, kind record.Kind, text string) bool {
	for _, r := range m.room.HistoryRecords() {
		if r.Kind != kind {
			continue
		}
		if strings.Contains(r.Text, text) {
			return true
		}
	}
	return false
}
