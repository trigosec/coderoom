package history

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	"github.com/trigosec/coderoom/internal/agent"
	rec "github.com/trigosec/coderoom/internal/ui/room/history/record"
)

// streamSlot tracks an open streaming record by its StreamID.
type streamSlot struct {
	recordIdx int
}

// Model holds the conversation record list and its viewport.
type Model struct {
	viewport      viewport.Model
	records       []rec.Record
	contentLines  int
	streaming     map[agent.StreamID]streamSlot // streamID → open record slot
	departed      map[string]bool
	debugRowNums  bool
	ready         bool
	colorByAlias  func(string) string
	departedColor string
	colorVersion  uint64
}

// ScrollStats summarizes the history viewport scroll position using the same
// wrapped content the viewport renders.
type ScrollStats struct {
	Top          int
	ViewportRows int
	ContentRows  int
	AtBottom     bool
}

// New returns an uninitialised Model; call SetSize before first use.
// colorByAlias resolves an active agent alias to its colour; it may be nil.
// departedColor is applied to records belonging to agents that have left.
func New(colorByAlias func(string) string, departedColor string) Model {
	return Model{
		records:       []rec.Record{},
		streaming:     make(map[agent.StreamID]streamSlot),
		departed:      make(map[string]bool),
		colorByAlias:  colorByAlias,
		departedColor: departedColor,
	}
}

// ScrollStats reports the current scroll position and content height.
func (m Model) ScrollStats() ScrollStats {
	viewportRows := m.viewport.Height()
	contentRows := m.contentLines
	top := m.viewport.YOffset()

	if viewportRows < 0 {
		viewportRows = 0
	}
	if contentRows < 0 {
		contentRows = 0
	}
	if top < 0 {
		top = 0
	}
	maxTop := 0
	if viewportRows > 0 && contentRows > viewportRows {
		maxTop = contentRows - viewportRows
	}
	if top > maxTop {
		top = maxTop
	}

	atBottom := true
	if viewportRows > 0 && contentRows > 0 {
		atBottom = top+viewportRows >= contentRows
	}
	return ScrollStats{
		Top:          top,
		ViewportRows: viewportRows,
		ContentRows:  contentRows,
		AtBottom:     atBottom,
	}
}

func countContentLines(s string) int {
	if s == "" {
		return 0
	}
	// Normalise to a content string without a trailing newline.
	s = strings.TrimSuffix(s, "\n")
	return strings.Count(s, "\n") + 1
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

func (m Model) viewportRenderContext() rec.RenderContext {
	return rec.RenderContext{
		Key: rec.RenderKey{
			Mode:         rec.RenderViewport,
			Width:        m.viewport.Width(),
			ColorVersion: m.colorVersion,
		},
		ColorForAlias: m.resolveColor,
	}
}

// Ready reports whether SetSize has been called at least once.
func (m Model) Ready() bool { return m.ready }

// Records returns the current record slice.
func (m Model) Records() []rec.Record { return m.records }

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
		r := m.records[slot.recordIdx]
		if r.Alias != alias || r.Msg == nil {
			continue
		}
		if _, ok := r.Msg.Content.(agent.Output); ok {
			return slot.recordIdx, true
		}
	}
	return 0, false
}

// IsDeparted reports whether alias has left or crashed.
func (m Model) IsDeparted(alias string) bool { return m.departed[alias] }

// Height returns the current viewport height.
func (m Model) Height() int { return m.viewport.Height() }

// Width returns the current viewport width.
func (m Model) Width() int { return m.viewport.Width() }

// ToggleDebugRowNums flips the row-number overlay.
func (m Model) ToggleDebugRowNums() Model {
	m.debugRowNums = !m.debugRowNums
	return m
}
