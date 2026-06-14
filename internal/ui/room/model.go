// Package room composes the in-room UI: history viewport + compose input.
//
// It owns focus and layout between these components, but it does not interpret
// submitted text as commands/actions. The parent UI decides what to do with a
// submission.
package room

import (
	tea "charm.land/bubbletea/v2"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/ui/room/approval"
	"github.com/trigosec/coderoom/internal/ui/room/compose"
	"github.com/trigosec/coderoom/internal/ui/room/history"
	rec "github.com/trigosec/coderoom/internal/ui/room/history/record"
	"github.com/trigosec/coderoom/internal/ui/room/staging"
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
	inputStaged
)

type inputModel struct {
	kind     inputKind
	compose  compose.Model
	approval approval.Model
	staged   stagedInput
}

func (m inputModel) height(totalH int) int {
	if m.kind == inputApproval {
		// Approval view is variable; keep it to 6 lines max and at least 3.
		return min(6, max(totalH/3, 3))
	}
	return m.compose.Height()
}

type stagedInput struct {
	status string
	batch  *staging.Batch
}

// SubmitMsg is emitted when the user submits the composer (Enter without Alt).
// The parent should handle the text (parse, route, execute) and update the room
// history accordingly.
type SubmitMsg struct {
	Text string
}

// StagedEditMsg is emitted when the user presses Esc while the composer is
// staged, returning to draft mode with the staged text loaded for editing.
type StagedEditMsg struct{}

// StagedInterruptMsg is emitted when the user presses Ctrl+X while the composer
// is staged, requesting an interrupt-and-send.
type StagedInterruptMsg struct{}

// StagedClearMsg is emitted when the user clears a staged composer (Ctrl+C).
type StagedClearMsg struct{}

// ApprovalDecisionMsg is emitted when the user confirms an approval option.
// The parent is responsible for forwarding this decision to the active
// ApprovalListener and resuming the agent.
type ApprovalDecisionMsg struct {
	Choice agent.ApprovalOption
}

// Model is the Bubble Tea component for a single room: history + composer.
type Model struct {
	history      history.Model
	input        inputModel
	approvalPrev inputKind
	focus        roomFocus
	debug        bool
	lastSize     tea.WindowSizeMsg
	colorByAlias func(string) string
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
		focus:        focusInput,
		colorByAlias: colorByAlias,
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

// IsComposerStaged reports whether the composer is currently staged (read-only).
func (m Model) IsComposerStaged() bool { return m.input.kind == inputStaged }

// SetComposerStaged enables staged mode with the given text and status line.
// The compose buffer is shown read-only and keystrokes are blocked until the
// user exits staged mode.
func (m Model) SetComposerStaged(text string, status string) Model {
	m.input.kind = inputStaged
	m.input.staged.status = status
	m.input.compose = m.input.compose.SetValue(text).Blur()
	if m.lastSize.Width > 0 && m.lastSize.Height > 0 {
		m = m.HandleResize(m.lastSize.Width, m.lastSize.Height)
	}
	return m
}

// SetComposerStagedStatus updates the staged status line without changing
// staged text.
func (m Model) SetComposerStagedStatus(status string) Model {
	if m.input.kind != inputStaged {
		return m
	}
	m.input.staged.status = status
	if m.lastSize.Width > 0 && m.lastSize.Height > 0 {
		m = m.HandleResize(m.lastSize.Width, m.lastSize.Height)
	}
	return m
}

// ClearComposerStaged exits staged mode and clears the status line.
func (m Model) ClearComposerStaged() Model {
	if m.input.kind != inputStaged {
		return m
	}
	m.input.kind = inputCompose
	m.input.staged.status = ""
	m.input.staged.batch = nil
	// If we blurred the textarea while staged, ensure we restore focus when the
	// input area is focused; otherwise the composer becomes uneditable after
	// auto-dispatch clears staging.
	if m.focus == focusInput {
		m.input.compose, _ = m.input.compose.Focus()
	}
	if m.lastSize.Width > 0 && m.lastSize.Height > 0 {
		m = m.HandleResize(m.lastSize.Width, m.lastSize.Height)
	}
	return m
}

// HasStagedBatch reports whether a staged barrier-batch is currently active.
func (m Model) HasStagedBatch() bool { return m.input.staged.batch != nil }

// StagedBatch returns the current staged batch, or nil if none is staged.
func (m Model) StagedBatch() *staging.Batch { return m.input.staged.batch }

// StageBatch stages the batch and switches the composer into staged (read-only)
// mode, updating the status line based on blocked participants.
func (m Model) StageBatch(b *staging.Batch, blocked []string) Model {
	m.input.staged.batch = b
	status := staging.RenderStatusLine(b, blocked, m.colorByAlias)
	return m.SetComposerStaged(b.Raw, status)
}

// UpdateStagedStatus recomputes the staged status line based on blocked
// participants. It is a no-op when no batch is staged.
func (m Model) UpdateStagedStatus(blocked []string) Model {
	if m.input.staged.batch == nil {
		return m
	}
	status := staging.RenderStatusLine(m.input.staged.batch, blocked, m.colorByAlias)
	return m.SetComposerStagedStatus(status)
}

// DispatchStagedBatch appends the staged user input record, clears staged mode,
// and returns the staged action and routing targets. It is a no-op when no
// batch is staged.
func (m Model) DispatchStagedBatch() (Model, staging.Action, []string) {
	act, targets, ok := m.StagedDispatchCandidate()
	if !ok {
		return m, staging.Action{Kind: staging.ActionUnknown}, nil
	}
	m = m.CommitStagedBatchDispatch(targets)
	return m, act, targets
}

// StagedDispatchCandidate returns the current staged action and active targets
// without mutating room state.
func (m Model) StagedDispatchCandidate() (staging.Action, []string, bool) {
	if m.input.staged.batch == nil {
		return staging.Action{Kind: staging.ActionUnknown}, nil, false
	}
	return m.input.staged.batch.Action, m.input.staged.batch.ActiveTargets(), true
}

// CommitStagedBatchDispatch records the staged user input and clears staged
// mode after dispatch has successfully started.
func (m Model) CommitStagedBatchDispatch(targets []string) Model {
	if m.input.staged.batch == nil {
		return m
	}
	m = m.AppendUserInput(m.input.staged.batch.Raw, targets)
	m = m.ClearComposerStaged()
	m = m.SetComposeValue("")
	return m
}

// MarkStagedInterruptRequested marks the staged batch as "interrupt requested".
func (m Model) MarkStagedInterruptRequested() Model {
	if m.input.staged.batch == nil {
		return m
	}
	m.input.staged.batch.Interrupt = true
	return m
}

// MarkStagedDiscarded marks a target as discarded for the current batch.
func (m Model) MarkStagedDiscarded(alias string) Model {
	if m.input.staged.batch == nil {
		return m
	}
	m.input.staged.batch.MarkDiscarded(alias)
	return m
}

// StagedBlockedTargets returns the currently blocked targets for the staged
// batch using the provided status resolver.
func (m Model) StagedBlockedTargets(statusByAlias func(alias string) (participant.Status, bool)) []string {
	if m.input.staged.batch == nil {
		return nil
	}
	return m.input.staged.batch.BlockedTargets(statusByAlias)
}

// StageBatchOrDispatch stages the given batch and returns shouldDispatch=true
// if no participants are blocked (meaning the batch can be dispatched
// immediately).
func (m Model) StageBatchOrDispatch(b *staging.Batch, statusByAlias func(alias string) (participant.Status, bool)) (next Model, shouldDispatch bool) {
	blocked := b.BlockedTargets(statusByAlias)
	m = m.StageBatch(b, blocked)
	return m, len(blocked) == 0
}

// RefreshStagedStatus recomputes blocked targets and updates the staged status
// line. It returns shouldDispatch=true if the batch is now unblocked.
func (m Model) RefreshStagedStatus(statusByAlias func(alias string) (participant.Status, bool)) (next Model, shouldDispatch bool) {
	if m.input.staged.batch == nil {
		return m, false
	}
	blocked := m.input.staged.batch.BlockedTargets(statusByAlias)
	if len(blocked) == 0 {
		return m, true
	}
	m = m.UpdateStagedStatus(blocked)
	return m, false
}

// RequestStagedInterrupt sets Interrupt=true on the staged batch and returns
// the currently blocked targets. The caller is responsible for issuing any
// side-effectful interrupts/cancels.
func (m Model) RequestStagedInterrupt(statusByAlias func(alias string) (participant.Status, bool)) (next Model, blocked []string, shouldDispatch bool) {
	if m.input.staged.batch == nil {
		return m, nil, false
	}
	m = m.MarkStagedInterruptRequested()
	blocked = m.input.staged.batch.BlockedTargets(statusByAlias)
	if len(blocked) == 0 {
		return m, nil, true
	}
	m = m.UpdateStagedStatus(blocked)
	return m, blocked, false
}

// ShowApproval switches the input area to an approval prompt.
func (m Model) ShowApproval(req agent.ApprovalRequest) Model {
	if m.input.kind != inputApproval {
		m.approvalPrev = m.input.kind
	}
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
	m.input.kind = m.approvalPrev
	m.approvalPrev = inputCompose
	m.focus = focusInput
	if m.lastSize.Width > 0 && m.lastSize.Height > 0 {
		m = m.HandleResize(m.lastSize.Width, m.lastSize.Height)
	}
	if m.input.kind == inputCompose {
		return m.composeFocus()
	}
	return m, nil
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
	wasAtBottom := false
	if m.history.Ready() {
		wasAtBottom = m.history.ScrollStats().AtBottom
	}
	m.lastSize = tea.WindowSizeMsg{Width: innerW, Height: totalH}
	m.input.compose = m.input.compose.SetWidth(innerW).SetMaxHeightFromTotal(totalH)
	// Layout:
	//   header (1 line)
	//   history (variable height)
	//   top separator (1 line)
	//   input (either composer or approval view; composer height is dynamic)
	//   bottom separator (1 line)
	inputH := m.input.height(totalH)
	if m.input.kind == inputStaged && m.input.staged.status != "" {
		inputH++
	}
	h := max(totalH-(3+inputH), 1)
	m.history = m.history.SetSize(innerW, h)
	m.history = m.history.RebuildColors()
	if wasAtBottom {
		m.history = m.history.GotoBottom()
	}
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
