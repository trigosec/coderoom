// Package approval implements the approval prompt input as a Bubble Tea component.
package approval

import (
	"github.com/trigosec/coderoom/internal/agent"
)

// Model holds the approval prompt and selection state.
type Model struct {
	ask      string
	options  []agent.ApprovalOption
	selected int
}

// New returns an empty Model.
func New() Model { return Model{} }

// Active reports whether an approval is currently being displayed.
func (m Model) Active() bool { return m.ask != "" && len(m.options) > 0 }

// Ask returns the prompt string.
func (m Model) Ask() string { return m.ask }

// Options returns the available options.
func (m Model) Options() []agent.ApprovalOption { return m.options }

// Selected returns the currently selected option index.
func (m Model) Selected() int { return m.selected }

// SelectedOption returns the currently selected option, if any.
func (m Model) SelectedOption() (agent.ApprovalOption, bool) {
	if m.selected < 0 || m.selected >= len(m.options) {
		return "", false
	}
	return m.options[m.selected], true
}

// Set sets the approval prompt and options and resets selection to 0.
func (m Model) Set(req agent.ApprovalRequest) Model {
	m.ask = req.Ask
	m.options = append([]agent.ApprovalOption(nil), req.Options...)
	m.selected = 0
	return m
}

// Clear removes the current approval.
func (m Model) Clear() Model {
	m.ask = ""
	m.options = nil
	m.selected = 0
	return m
}
