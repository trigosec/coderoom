package compose

import (
	"reflect"

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
		// bubbles/textarea clipboard paste arrives as an internal textarea.pasteMsg
		// (not a KeyMsg). Detect it and feed it rune-by-rune to avoid viewport
		// jumps that can clip wrapped rows.
		if isTextareaPasteMsg(msg) {
			s, ok := textareaPasteString(msg)
			if ok {
				var cmds []tea.Cmd
				for _, r := range s {
					m = m.recalcHeight()
					var cmd tea.Cmd
					m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
					cmds = append(cmds, cmd)
					m = m.recalcHeight()
				}
				return m, tea.Batch(cmds...)
			}
		}

		// Paste events and other textarea-driven messages arrive here (not as
		// KeyMsg). Recalculate height before and after so the textarea's internal
		// viewport scrolling decisions use the correct height.
		m = m.recalcHeight()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m.recalcHeight(), cmd
	}
}

func isTextareaPasteMsg(msg tea.Msg) bool {
	t := reflect.TypeOf(msg)
	if t == nil {
		return false
	}
	// textarea defines: type pasteMsg string
	return t.Kind() == reflect.String && t.Name() == "pasteMsg" && t.PkgPath() == "github.com/charmbracelet/bubbles/textarea"
}

func textareaPasteString(msg tea.Msg) (string, bool) {
	t := reflect.TypeOf(msg)
	if t == nil || t.Kind() != reflect.String {
		return "", false
	}
	// Safe conversion: underlying kind is string.
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.String {
		return "", false
	}
	return v.String(), true
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
		// Only apply rune-by-rune for actual pastes. IMEs can also emit multi-rune
		// KeyRunes, and per-rune updates make input feel sluggish.
		if msg.Type == tea.KeyRunes && msg.Paste && len(msg.Runes) > 1 {
			var cmds []tea.Cmd
			for _, r := range msg.Runes {
				m = m.recalcHeight()
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				cmds = append(cmds, cmd)
				m = m.recalcHeight()
			}
			return m, tea.Batch(cmds...)
		}

		m = m.recalcHeight()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m.recalcHeight(), cmd
	}
}
