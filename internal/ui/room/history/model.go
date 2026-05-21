package history

import (
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/trigosec/coderoom/internal/agent"
)

// streamSlot tracks an open streaming record by its StreamID.
type streamSlot struct {
	recordIdx int
	msg       agent.Message // accumulates fragments via Message.Accumulate
}

// Model holds the conversation record list and its viewport.
type Model struct {
	viewport        viewport.Model
	records         []Record
	renderedRecords []string
	streaming       map[agent.StreamID]streamSlot // streamID → open record slot
	departed        map[string]bool
	debugRowNums    bool
	ready           bool
	colorByAlias    func(string) string
	departedColor   string
}

// New returns an uninitialised Model; call SetSize before first use.
// colorByAlias resolves an active agent alias to its colour; it may be nil.
// departedColor is applied to records belonging to agents that have left.
func New(colorByAlias func(string) string, departedColor string) Model {
	return Model{
		records:         []Record{},
		renderedRecords: []string{},
		streaming:       make(map[agent.StreamID]streamSlot),
		departed:        make(map[string]bool),
		colorByAlias:    colorByAlias,
		departedColor:   departedColor,
	}
}

// resolveColor returns the display colour for alias, accounting for departed state.
func (m Model) resolveColor(alias string) string {
	if m.departed[alias] {
		return m.departedColor
	}
	if m.colorByAlias != nil {
		return m.colorByAlias(alias)
	}
	return ""
}

// Ready reports whether SetSize has been called at least once.
func (m Model) Ready() bool { return m.ready }

// Records returns the current record slice.
func (m Model) Records() []Record { return m.records }

// IsStreaming reports whether alias currently has any open stream.
func (m Model) IsStreaming(alias string) bool {
	for _, slot := range m.streaming {
		if m.records[slot.recordIdx].Alias == alias {
			return true
		}
	}
	return false
}

// StreamingIdx returns the record index for the open output stream of alias.
func (m Model) StreamingIdx(alias string) (int, bool) {
	for _, slot := range m.streaming {
		if _, ok := slot.msg.Content.(agent.Output); ok {
			if m.records[slot.recordIdx].Alias == alias {
				return slot.recordIdx, true
			}
		}
	}
	return 0, false
}

// IsDeparted reports whether alias has left or crashed.
func (m Model) IsDeparted(alias string) bool { return m.departed[alias] }

// Height returns the current viewport height.
func (m Model) Height() int { return m.viewport.Height }

// Width returns the current viewport width.
func (m Model) Width() int { return m.viewport.Width }

// ToggleDebugRowNums flips the row-number overlay.
func (m Model) ToggleDebugRowNums() Model {
	m.debugRowNums = !m.debugRowNums
	return m
}
