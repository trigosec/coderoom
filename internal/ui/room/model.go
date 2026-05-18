// Package room composes the in-room UI: history viewport + compose input.
//
// It owns focus and layout between these components, but it does not interpret
// submitted text as commands/actions. The parent UI decides what to do with a
// submission.
package room

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/ui/room/compose"
	"github.com/trigosec/coderoom/internal/ui/room/history"
)

type focusTarget int

const (
	focusComposer focusTarget = iota
	focusHistory
)

// SubmitMsg is emitted when the user submits the composer (Enter without Alt).
// The parent should handle the text (parse, route, execute) and update the room
// history accordingly.
type SubmitMsg struct {
	Text string
}

// Model is the Bubble Tea component for a single room: history + composer.
type Model struct {
	history  history.Model
	compose  compose.Model
	focus    focusTarget
	debug    bool
	lastSize tea.WindowSizeMsg
}

// New creates a room model with a fresh history model.
// colorByAlias resolves an active agent alias to its color; it may be nil.
// departedColor is used for departed agents.
func New(colorByAlias func(string) string, departedColor string) Model {
	return Model{
		history: history.New(colorByAlias, departedColor),
		compose: compose.New(),
		focus:   focusComposer,
	}
}

// Init returns the initial command for the component.
func (m Model) Init() tea.Cmd { return m.compose.Init() }

// Ready reports whether HandleResize has been called at least once.
func (m Model) Ready() bool { return m.history.Ready() }

// Width returns the current room inner width.
func (m Model) Width() int { return m.history.Width() }

// HistoryView renders the history viewport.
func (m Model) HistoryView() string { return m.history.View() }

// HistoryRecords returns the room's current record slice.
func (m Model) HistoryRecords() []history.Record { return m.history.Records() }

// HistoryRenderedContent returns the raw rendered record content joined by newlines.
func (m Model) HistoryRenderedContent() string { return m.history.RenderedContent() }

// HistoryPlainText returns a plain-text transcript of the rendered records.
func (m Model) HistoryPlainText() string { return m.history.PlainText() }

// HistoryHeight returns the history viewport height.
func (m Model) HistoryHeight() int { return m.history.Height() }

// IsStreaming reports whether alias currently has an open turn.
func (m Model) IsStreaming(alias string) bool { return m.history.IsStreaming(alias) }

// StreamingIdx returns the record index for the given streaming alias.
func (m Model) StreamingIdx(alias string) (int, bool) { return m.history.StreamingIdx(alias) }

// IsDeparted reports whether alias has left or crashed.
func (m Model) IsDeparted(alias string) bool { return m.history.IsDeparted(alias) }

// ComposeValue returns the current composer buffer.
func (m Model) ComposeValue() string { return m.compose.Value() }

// ComposeHeight returns the current composer height.
func (m Model) ComposeHeight() int { return m.compose.Height() }

// SetComposeValue replaces the composer buffer and updates layout.
func (m Model) SetComposeValue(s string) Model {
	m.compose = m.compose.SetValue(s)
	return m.syncAfterCompose()
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
	m.compose = m.compose.SetWidth(innerW).SetMaxHeightFromTotal(totalH)
	// Layout:
	//   history (variable height)
	//   separator (1 line)
	//   composer (m.compose.Height)
	h := max(totalH-(1+m.compose.Height()), 1)
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

// HandleDelta appends streaming output for alias.
func (m Model) HandleDelta(alias, text string) Model {
	m.history = m.history.HandleDelta(alias, text)
	return m
}

// HandleDone marks streaming as completed for alias.
func (m Model) HandleDone(alias string) Model {
	m.history = m.history.ClearStreaming(alias)
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
