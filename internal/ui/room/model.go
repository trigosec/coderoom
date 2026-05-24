// Package room composes the in-room UI: history viewport + compose input.
//
// It owns focus and layout between these components, but it does not interpret
// submitted text as commands/actions. The parent UI decides what to do with a
// submission.
package room

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/ui/room/approval"
	"github.com/trigosec/coderoom/internal/ui/room/compose"
	"github.com/trigosec/coderoom/internal/ui/room/history"
	rec "github.com/trigosec/coderoom/internal/ui/room/history/record"
)

type roomFocus int

const (
	focusInput roomFocus = iota
	focusHistory
)

type inputKind int

const (
	inputCompose inputKind = iota
	inputApproval
)

type inputModel struct {
	kind     inputKind
	compose  compose.Model
	approval approval.Model
}

func (m inputModel) height(totalH int) int {
	if m.kind == inputApproval {
		// Approval view is variable; keep it to 6 lines max and at least 3.
		return min(6, max(totalH/3, 3))
	}
	return m.compose.Height()
}

// SubmitMsg is emitted when the user submits the composer (Enter without Alt).
// The parent should handle the text (parse, route, execute) and update the room
// history accordingly.
type SubmitMsg struct {
	Text string
}

// ApprovalDecisionMsg is emitted when the user confirms an approval option.
// The parent is responsible for forwarding this decision to the active
// ApprovalListener and resuming the agent.
type ApprovalDecisionMsg struct {
	Choice agent.ApprovalOption
}

// Model is the Bubble Tea component for a single room: history + composer.
type Model struct {
	history  history.Model
	input    inputModel
	focus    roomFocus
	debug    bool
	lastSize tea.WindowSizeMsg
}

// New creates a room model with a fresh history model.
// colorByAlias resolves an active agent alias to its color; it may be nil.
// departedColor is used for departed agents.
func New(colorByAlias func(string) string, departedColor string) Model {
	compose := compose.New()
	return Model{
		history: history.New(colorByAlias, departedColor),
		input: inputModel{
			kind:     inputCompose,
			compose:  compose,
			approval: approval.New(),
		},
		focus: focusInput,
	}
}

// Init returns the initial command for the component.
func (m Model) Init() tea.Cmd { return m.input.compose.Init() }

// Ready reports whether HandleResize has been called at least once.
func (m Model) Ready() bool { return m.history.Ready() }

// Width returns the current room inner width.
func (m Model) Width() int { return m.history.Width() }

// HistoryView renders the history viewport.
func (m Model) HistoryView() string { return m.history.View() }

// HistoryRecords returns the room's current record slice.
func (m Model) HistoryRecords() []rec.Record { return m.history.Records() }

// HistoryRenderedContent returns the raw rendered record content joined by newlines.
func (m Model) HistoryRenderedContent() string { return m.history.RenderedContent() }

// HistoryPlainText returns a plain-text transcript of the rendered records.
func (m Model) HistoryPlainText() string { return m.history.PlainText() }

// HistoryHeight returns the history viewport height.
func (m Model) HistoryHeight() int { return m.history.Height() }

// IsStreaming reports whether alias currently has an open turn.
func (m Model) IsStreaming(alias string) bool { return m.history.IsStreaming(alias) }

// IsReasoningStreaming reports whether alias currently has an open reasoning record.
func (m Model) IsReasoningStreaming(alias string) bool {
	return m.history.IsReasoningStreaming(alias)
}

// StreamingIdx returns the record index for the given streaming alias.
func (m Model) StreamingIdx(alias string) (int, bool) { return m.history.StreamingIdx(alias) }

// IsDeparted reports whether alias has left or crashed.
func (m Model) IsDeparted(alias string) bool { return m.history.IsDeparted(alias) }

// ComposeValue returns the current composer buffer.
func (m Model) ComposeValue() string { return m.input.compose.Value() }

// ComposeHeight returns the current composer height.
func (m Model) ComposeHeight() int { return m.input.compose.Height() }

// SetComposeValue replaces the composer buffer and updates layout.
func (m Model) SetComposeValue(s string) Model {
	m.input.compose = m.input.compose.SetValue(s)
	return m.syncAfterCompose()
}

// ShowApproval switches the input area to an approval prompt.
func (m Model) ShowApproval(req agent.ApprovalRequest) Model {
	m.input.approval = m.input.approval.Set(req)
	m.input.kind = inputApproval
	m.focus = focusInput
	m.input.compose = m.input.compose.Blur()
	if m.lastSize.Width > 0 && m.lastSize.Height > 0 {
		m = m.HandleResize(m.lastSize.Width, m.lastSize.Height)
	}
	return m
}

// ClearApproval returns to compose input mode and clears the approval prompt.
func (m Model) ClearApproval() (Model, tea.Cmd) {
	m.input.approval = m.input.approval.Clear()
	m.input.kind = inputCompose
	m.focus = focusInput
	if m.lastSize.Width > 0 && m.lastSize.Height > 0 {
		m = m.HandleResize(m.lastSize.Width, m.lastSize.Height)
	}
	return m.composeFocus()
}

func (m Model) composeFocus() (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input.compose, cmd = m.input.compose.Focus()
	return m, cmd
}

// SetDebug enables or disables debug features on the room.
func (m Model) SetDebug(enabled bool) Model {
	m.debug = enabled
	return m
}

// ToggleDebugRowNums toggles the history row-number overlay.
func (m Model) ToggleDebugRowNums() Model {
	m.history = m.history.ToggleDebugRowNums()
	return m
}

// HandleResize sets the component sizes.
//
// totalH is the height available to the room (it excludes any outer chrome like
// the toolbox and bottom padding).
func (m Model) HandleResize(innerW, totalH int) Model {
	m.lastSize = tea.WindowSizeMsg{Width: innerW, Height: totalH}
	m.input.compose = m.input.compose.SetWidth(innerW).SetMaxHeightFromTotal(totalH)
	// Layout:
	//   header (1 line)
	//   history (variable height)
	//   top separator (1 line)
	//   input (either composer or approval view; composer height is dynamic)
	//   bottom separator (1 line)
	inputH := m.input.height(totalH)
	h := max(totalH-(3+inputH), 1)
	m.history = m.history.SetSize(innerW, h)
	m.history = m.history.RebuildColors()
	return m
}

// AppendUserInput appends a user input record to history.
func (m Model) AppendUserInput(body string, routing []string) Model {
	m.history = m.history.AppendUserInputRecord(body, routing)
	return m
}

// AppendSystem appends a system record to history.
func (m Model) AppendSystem(text string) Model {
	m.history = m.history.AppendSystemRecord(text)
	return m
}

// AppendLog appends a log record to history.
func (m Model) AppendLog(alias, text string) Model {
	m.history = m.history.AppendLogRecord(alias, text)
	return m
}

// HandleAgentMessage routes an agent message to history for streaming record management.
func (m Model) HandleAgentMessage(alias string, msg agent.Message) Model {
	m.history = m.history.HandleAgentMessage(alias, msg)
	return m
}

// MarkDeparted marks alias as departed and recolors its affected records.
func (m Model) MarkDeparted(alias string) Model {
	m.history = m.history.ClearStreaming(alias)
	m.history = m.history.MarkDeparted(alias)
	return m
}

// GotoBottom scrolls history to the bottom.
func (m Model) GotoBottom() Model {
	m.history = m.history.GotoBottom()
	return m
}

// DebugLabel returns the separator label suffix used in debug mode.
func (m Model) DebugLabel() string { return m.history.DebugLabel() }

// HistoryDebugSummary returns a multi-line summary of the history viewport.
func (m Model) HistoryDebugSummary() string { return m.history.DebugSummary() }

// AtBottom reports whether the history viewport is at the bottom.
func (m Model) AtBottom() bool { return m.history.AtBottom() }

// YOffset returns the history viewport vertical offset.
func (m Model) YOffset() int { return m.history.YOffset() }
