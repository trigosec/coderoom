package approval

import (
	tea "github.com/charmbracelet/bubbletea"
)

// ConfirmMsg is emitted when the user confirms the currently selected option.
type ConfirmMsg struct{}

// CancelMsg is emitted when the user cancels/dismisses the approval prompt.
type CancelMsg struct{}

// Update handles key navigation for approvals.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok || !m.Active() {
		return m, nil
	}
	return m.handleKey(key)
}

func (m Model) handleKey(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyUp:
		return m.move(-1), nil
	case tea.KeyDown:
		return m.move(1), nil
	case tea.KeyRunes:
		return m.handleRunes(key.Runes)
	case tea.KeyEnter:
		return m, func() tea.Msg { return ConfirmMsg{} }
	case tea.KeyEsc:
		return m, func() tea.Msg { return CancelMsg{} }
	default:
		return m, nil
	}
}

func (m Model) handleRunes(runes []rune) (Model, tea.Cmd) {
	if len(runes) != 1 {
		return m, nil
	}
	switch runes[0] {
	case 'k':
		return m.move(-1), nil
	case 'j':
		return m.move(1), nil
	default:
		return m, nil
	}
}

func (m Model) move(delta int) Model {
	n := len(m.options)
	if n == 0 {
		return m
	}
	m.selected = (m.selected + delta) % n
	if m.selected < 0 {
		m.selected += n
	}
	return m
}
