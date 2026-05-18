package compose

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textarea"
)

// View renders the input area.
func (m Model) View() string {
	return m.input.View()
}

func applyDecorations(input textarea.Model) textarea.Model {
	if input.LineCount() >= 2 {
		input.ShowLineNumbers = false
		input.SetPromptFunc(6, func(lineIndex int) string {
			return fmt.Sprintf("❯%4d ", lineIndex+1)
		})
		return input
	}
	input.ShowLineNumbers = false
	input.SetPromptFunc(6, func(_ int) string {
		return "❯     "
	})
	return input
}
