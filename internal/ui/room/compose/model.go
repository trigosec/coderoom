// Package compose implements the text-input area as a Bubble Tea component.
package compose

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// Model holds the textarea and its height constraints.
type Model struct {
	input textarea.Model
	maxH  int
}

// New creates a focused Model with default decorations applied.
func New() Model {
	ti := textarea.New()
	ti.ShowLineNumbers = false
	ti.KeyMap.WordForward = key.NewBinding(key.WithKeys("alt+right", "alt+f", "ctrl+right"))
	ti.KeyMap.WordBackward = key.NewBinding(key.WithKeys("alt+left", "alt+b", "ctrl+left"))
	ti = applyDecorations(ti)
	ti.Focus()
	return Model{input: ti}
}

// Init returns the textarea cursor-blink command.
func (m Model) Init() tea.Cmd { return textarea.Blink }

// SetWidth sets the display width.
func (m Model) SetWidth(w int) Model {
	m.input.SetWidth(w)
	return m
}

// SetMaxHeightFromTotal derives the input height cap from the total terminal
// height and recalculates the current input height.
func (m Model) SetMaxHeightFromTotal(totalH int) Model {
	m.maxH = inputMaxHeight(totalH)
	return m.recalcHeight()
}

// Height returns the current rendered height of the input area.
func (m Model) Height() int { return m.input.Height() }

// Value returns the current text content.
func (m Model) Value() string { return m.input.Value() }

// SetValue replaces the text content and recalculates height.
func (m Model) SetValue(s string) Model {
	m.input.SetValue(s)
	return m.recalcHeight()
}

// Reset clears the text content and recalculates height.
func (m Model) Reset() Model {
	m.input.Reset()
	return m.recalcHeight()
}

// Focus focuses the textarea cursor.
func (m Model) Focus() (Model, tea.Cmd) {
	cmd := m.input.Focus()
	return m, cmd
}

// Blur removes focus from the textarea.
func (m Model) Blur() Model {
	m.input.Blur()
	return m
}

func inputMaxHeight(totalH int) int {
	return min(8, max(totalH/3, 1))
}

func desiredInputHeight(lineCount, maxH int) int {
	return min(max(lineCount, 1), maxH)
}

func (m Model) recalcHeight() Model {
	m.input = applyDecorations(m.input)
	if m.maxH > 0 {
		m.input.SetHeight(desiredInputHeight(m.input.LineCount(), m.maxH))
	}
	return m
}
