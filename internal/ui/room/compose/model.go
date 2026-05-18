// Package compose implements the text-input area as a Bubble Tea component.
package compose

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rivo/uniseg"
)

// Model holds the textarea and its height constraints.
type Model struct {
	input     textarea.Model
	maxH      int
	visH      int // total visual rows (cached)
	scrollOff int // approximate first visible visual row (cached)
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

// HasAbove reports whether content is scrolled above the visible area.
func (m Model) HasAbove() bool { return m.scrollOff > 0 }

// HasBelow reports whether content extends below the visible area.
func (m Model) HasBelow() bool { return m.scrollOff+m.input.Height() < m.visH }

func (m Model) recalcHeight() Model {
	m.input = applyDecorations(m.input)
	if m.maxH > 0 {
		m.visH = visualRowCount(m.input.Value(), m.input.Width())
		m.input.SetHeight(min(max(m.visH, 1), m.maxH))
		m.scrollOff = m.approximateScrollOff()
	}
	return m
}

// approximateScrollOff estimates the first visible visual row by assuming the
// textarea has scrolled to keep the cursor at the bottom of the viewport. This
// is accurate when typing at the end of the buffer (the common case) and
// slightly over-estimates when the cursor is in the middle of the visible area.
func (m Model) approximateScrollOff() int {
	h := m.input.Height()
	if m.visH <= h {
		return 0
	}
	cursorLogLine := m.input.Line()
	contentW := m.input.Width() - promptWidth
	if contentW <= 0 {
		return max(0, cursorLogLine-h+1)
	}
	visual := 0
	idx := 0
	for line := range strings.SplitSeq(m.input.Value(), "\n") {
		w := uniseg.StringWidth(line)
		visual += max(1, (w+contentW-1)/contentW)
		if idx == cursorLogLine {
			break
		}
		idx++
	}
	return max(0, visual-h)
}

// visualRowCount returns the number of terminal rows needed to render text
// given totalWidth (the full textarea width including the prompt prefix).
func visualRowCount(text string, totalWidth int) int {
	contentW := totalWidth - promptWidth
	if contentW <= 0 {
		return max(1, strings.Count(text, "\n")+1)
	}
	total := 0
	for line := range strings.SplitSeq(text, "\n") {
		w := uniseg.StringWidth(line)
		rows := max(1, (w+contentW-1)/contentW)
		total += rows
	}
	return max(1, total)
}
