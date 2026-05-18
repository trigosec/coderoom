package toolbox

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type tickMsg time.Time

// Update handles the periodic tick that re-renders active participant glyphs.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if t, ok := msg.(tickMsg); ok {
		return m.handleTick(time.Time(t))
	}
	return m, nil
}

func (m Model) ensureTick() (Model, tea.Cmd) {
	if m.tickActive || !m.WantsTick() {
		return m, nil
	}
	m.tickActive = true
	return m, tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) handleTick(_ time.Time) (Model, tea.Cmd) {
	if !m.WantsTick() {
		m.tickActive = false
		return m, nil
	}
	return m, tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}
