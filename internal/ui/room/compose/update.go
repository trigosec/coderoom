package compose

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles messages. Ctrl+C clears the input; Alt+Enter inserts a
// newline; all other keys are delegated to the textarea.
// Enter without Alt and Ctrl+G are intentionally not handled here — they
// carry ui-level semantics (submit, open editor) and are intercepted by the
// parent before reaching this Update.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.input.Value() == "" {
			return m, nil
		}
		m.input.Reset()
		return m.recalcHeight(), nil
	case tea.KeyEnter:
		if msg.Alt {
			m.input.InsertRune('\n')
			return m.recalcHeight(), nil
		}
		// Non-Alt Enter is handled by the parent; treat as no-op here.
		return m, nil
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m.recalcHeight(), cmd
	}
}
