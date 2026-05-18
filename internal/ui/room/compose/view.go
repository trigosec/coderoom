package compose

import (
	"github.com/charmbracelet/bubbles/textarea"
)

const promptWidth = 2

// View renders the input area.
func (m Model) View() string {
	return m.input.View()
}

func applyDecorations(input textarea.Model) textarea.Model {
	input.ShowLineNumbers = false
	input.SetPromptFunc(promptWidth, func(lineIndex int) string {
		if lineIndex == 0 {
			return "❯ "
		}
		return "  "
	})
	return input
}
