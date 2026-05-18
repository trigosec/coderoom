package room

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/ui/editor"
	"github.com/trigosec/coderoom/internal/ui/room/approval"
)

// Update handles incoming messages and returns the next model state.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.input.kind == inputApproval {
		if next, cmd, ok := m.handleApprovalMessage(msg); ok {
			return next, cmd
		}
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		// Parent is expected to call HandleResize with a height already adjusted
		// for outer chrome; ignore direct WindowSizeMsg here.
		return m, nil
	case editor.Response:
		return m.handleEditorResult(msg), nil
	default:
		// Non-key messages (cursor blink, mouse events, etc).
		//
		// Compose needs these (e.g. cursor blink), but the approval input does not.
		var inputCmd tea.Cmd
		if m.input.kind == inputCompose {
			m.input.compose, inputCmd = m.input.compose.Update(msg)
			m = m.syncAfterCompose()
		}
		var historyCmd tea.Cmd
		m.history, historyCmd = m.history.Update(msg)
		return m, tea.Batch(inputCmd, historyCmd)
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlO {
		return m.toggleFocus()
	}

	// PgUp/PgDn always scroll history, regardless of focus.
	if msg.Type == tea.KeyPgUp {
		m.history = m.history.HalfPageUp()
		return m, nil
	}
	if msg.Type == tea.KeyPgDown {
		m.history = m.history.HalfPageDown()
		return m, nil
	}

	if m.focus == focusHistory {
		return m.handleHistoryKey(msg)
	}
	if m.input.kind == inputApproval {
		return m.handleApprovalKey(msg)
	}
	return m.handleComposeKey(msg)
}

func (m Model) toggleFocus() (Model, tea.Cmd) {
	if m.focus == focusInput {
		m.focus = focusHistory
		m.input.compose = m.input.compose.Blur()
		return m, nil
	}
	m.focus = focusInput
	if m.input.kind == inputCompose {
		return m.composeFocus()
	}
	return m, nil
}

func (m Model) handleApprovalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		// Treat Ctrl+C as cancel when an approval is active.
		next, cmd := m.ClearApproval()
		return next, cmd
	default:
		var cmd tea.Cmd
		m.input.approval, cmd = m.input.approval.Update(msg)
		// Confirmation/cancel is signaled via messages.
		return m, cmd
	}
}

func (m Model) handleComposeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlG {
		return m.startEditorCompose()
	}
	if msg.Type == tea.KeyEnter && !msg.Alt {
		// Reset immediately to keep the UI responsive. The parent receives the
		// submitted text via SubmitMsg and decides how to handle it.
		raw := m.input.compose.Value()
		m.input.compose = m.input.compose.Reset()
		m = m.syncAfterCompose()
		if strings.TrimSpace(raw) == "" {
			return m, nil
		}
		return m, func() tea.Msg { return SubmitMsg{Text: raw} }
	}
	var cmd tea.Cmd
	m.input.compose, cmd = m.input.compose.Update(msg)
	m = m.syncAfterCompose()
	return m, cmd
}

func (m Model) handleHistoryKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, nil
	case tea.KeyCtrlG:
		return m.openEditorWithTranscript()
	case tea.KeyUp:
		m.history = m.history.ScrollUp(1)
		return m, nil
	case tea.KeyDown:
		m.history = m.history.ScrollDown(1)
		return m, nil
	case tea.KeyHome:
		m.history = m.history.GotoTop()
		return m, nil
	case tea.KeyEnd:
		m.history = m.history.GotoBottom()
		return m, nil
	case tea.KeyEsc:
		m.focus = focusInput
		if m.input.kind == inputCompose {
			return m.composeFocus()
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.history, cmd = m.history.Update(msg)
		return m, cmd
	}
}

func (m Model) startEditorCompose() (Model, tea.Cmd) {
	prior := m.input.compose.Value()
	cmd, err := editor.OpenTempFileInEditor(editor.Request{
		Purpose:          editor.PurposeCompose,
		PriorText:        prior,
		InitialText:      prior,
		TempPattern:      "coderoom-compose-*.md",
		TrimFinalNewline: true,
	})
	if err != nil {
		m.history = m.history.AppendSystemRecord("error: " + err.Error())
		return m, nil
	}
	return m, cmd
}

func (m Model) handleEditorResult(msg editor.Response) Model {
	switch msg.Purpose {
	case editor.PurposeCompose:
		if msg.Canceled || msg.Err != nil {
			m.input.compose = m.input.compose.SetValue(msg.PriorText)
		} else {
			m.input.compose = m.input.compose.SetValue(msg.NewText)
		}
		return m.syncAfterCompose()
	case editor.PurposeTranscript:
		return m
	default:
		return m
	}
}

func (m Model) handleApprovalMessage(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg.(type) {
	case approval.ConfirmMsg:
		opt, ok := m.input.approval.SelectedOption()
		if !ok {
			next, focusCmd := m.ClearApproval()
			return next, focusCmd, true
		}
		next, focusCmd := m.ClearApproval()
		decisionCmd := func() tea.Msg { return ApprovalDecisionMsg{Choice: opt} }
		return next, tea.Batch(focusCmd, decisionCmd), true
	case approval.CancelMsg:
		choice := agent.OptionDecline
		if approvalHasOption(m.input.approval.Options(), agent.OptionCancel) {
			choice = agent.OptionCancel
		}
		next, focusCmd := m.ClearApproval()
		decisionCmd := func() tea.Msg { return ApprovalDecisionMsg{Choice: choice} }
		return next, tea.Batch(focusCmd, decisionCmd), true
	default:
		return m, nil, false
	}
}

func approvalHasOption(opts []agent.ApprovalOption, want agent.ApprovalOption) bool {
	for _, opt := range opts {
		if opt == want {
			return true
		}
	}
	return false
}

func (m Model) openEditorWithTranscript() (Model, tea.Cmd) {
	content := ansi.Strip(m.history.PlainText())
	cmd, err := editor.OpenTempFileInEditor(editor.Request{
		Purpose:     editor.PurposeTranscript,
		InitialText: content,
		TempPattern: "coderoom-transcript-*.txt",
		ReadOnly:    true,
	})
	if err != nil {
		m.history = m.history.AppendSystemRecord("error: " + err.Error())
		return m, nil
	}
	return m, cmd
}

// syncAfterCompose adjusts history height to match the current compose height,
// preserving the bottom anchor if history was already at bottom.
func (m Model) syncAfterCompose() Model {
	if !m.history.Ready() {
		return m
	}
	totalH := m.lastSize.Height
	if totalH <= 0 {
		return m
	}
	newHistH := max(totalH-(2+m.input.compose.Height()), 1)
	if newHistH == m.history.Height() {
		return m
	}
	wasAtBottom := m.history.AtBottom()
	m.history = m.history.SetHeight(newHistH)
	if wasAtBottom {
		m.history = m.history.GotoBottom()
	}
	return m
}
