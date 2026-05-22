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
					var cmd tea.Cmd
					m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
					cmds = append(cmds, cmd)
				}
				return m.recalcHeight(), tea.Batch(cmds...)
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
	if out, ok := m.handleCtrlCKey(msg); ok {
		return out, nil
	}
	if out, ok := m.handleEnterKey(msg); ok {
		return out, nil
	}
	if out, cmd, ok := m.handleNavKey(msg); ok {
		return out, cmd
	}
	return m.handleTextKey(msg)
}

func (m Model) handleCtrlCKey(msg tea.KeyMsg) (Model, bool) {
	if msg.Type != tea.KeyCtrlC {
		return m, false
	}
	if m.input.Value() == "" {
		return m, true
	}
	m.input.Reset()
	return m.recalcHeight(), true
}

func (m Model) handleEnterKey(msg tea.KeyMsg) (Model, bool) {
	if msg.Type != tea.KeyEnter {
		return m, false
	}
	if msg.Alt {
		m.input.InsertRune('\n')
		return m.recalcHeight(), true
	}
	// Non-Alt Enter is handled by the parent; treat as no-op here.
	return m, true
}

func (m Model) handleNavKey(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyUp:
		// When already on the first visual row of the buffer, Up moves to the
		// first character.
		li := m.input.LineInfo()
		if m.input.Line() == 0 && li.RowOffset == 0 && li.ColumnOffset > 0 {
			m.input.CursorStart()
			return m.recalcHeight(), nil, true
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m.recalcHeight(), cmd, true
	case tea.KeyDown:
		// When already on the last line, Down moves to the last character.
		lastLine := m.input.LineCount() - 1
		li := m.input.LineInfo()
		if m.input.Line() >= lastLine && li.RowOffset+1 >= li.Height {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyEnd})
			return m.recalcHeight(), cmd, true
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m.recalcHeight(), cmd, true
	default:
		return m, nil, false
	}
}

func (m Model) handleTextKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Only apply rune-by-rune for actual pastes. IMEs can also emit multi-rune
	// KeyRunes, and per-rune updates make input feel sluggish.
	if msg.Type == tea.KeyRunes && msg.Paste && len(msg.Runes) > 1 {
		var cmds []tea.Cmd
		for _, r := range msg.Runes {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			cmds = append(cmds, cmd)
		}
		return m.recalcHeight(), tea.Batch(cmds...)
	}

	m = m.recalcHeight()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m.recalcHeight(), cmd
}
