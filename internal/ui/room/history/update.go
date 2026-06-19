package history

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/trigosec/coderoom/internal/agent"
	rec "github.com/trigosec/coderoom/internal/ui/room/history/record"
)

// SetSize initialises or resizes the viewport.
func (m Model) SetSize(w, h int) Model {
	if !m.ready {
		m.viewport = viewport.New(viewport.WithWidth(w), viewport.WithHeight(h))
		m.ready = true
	} else {
		m.viewport.SetWidth(w)
		m.viewport.SetHeight(h)
	}
	return m.syncViewport()
}

// SetHeight adjusts the viewport height and re-syncs content.
func (m Model) SetHeight(h int) Model {
	m.viewport.SetHeight(h)
	return m.syncViewport()
}

// RebuildColors re-renders every record using the current color resolution.
func (m Model) RebuildColors() Model {
	m.colorVersion++
	return m.syncViewport()
}

// IsReasoningStreaming reports whether alias has an open reasoning stream.
func (m Model) IsReasoningStreaming(alias string) bool {
	for _, slot := range m.streaming {
		r := m.records[slot.recordIdx].record
		if r.Alias != alias || r.Msg == nil {
			continue
		}
		if _, ok := r.Msg.Content.(agent.Reasoning); ok {
			return true
		}
	}
	return false
}

// Update forwards the message to the viewport (handles mouse scroll, etc.).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// HalfPageUp scrolls the viewport up by half a page.
func (m Model) HalfPageUp() Model { m.viewport.HalfPageUp(); return m }

// HalfPageDown scrolls the viewport down by half a page.
func (m Model) HalfPageDown() Model { m.viewport.HalfPageDown(); return m }

// ScrollUp scrolls up by n lines.
func (m Model) ScrollUp(n int) Model { m.viewport.ScrollUp(n); return m }

// ScrollDown scrolls down by n lines.
func (m Model) ScrollDown(n int) Model { m.viewport.ScrollDown(n); return m }

// GotoTop scrolls to the top of the viewport.
func (m Model) GotoTop() Model { m.viewport.GotoTop(); return m }

// GotoBottom scrolls to the bottom of the viewport.
func (m Model) GotoBottom() Model { m.viewport.GotoBottom(); return m }

// AtBottom reports whether the viewport is at the bottom.
func (m Model) AtBottom() bool { return m.viewport.AtBottom() }

// YOffset returns the current viewport vertical scroll offset.
func (m Model) YOffset() int { return m.viewport.YOffset() }

func (m Model) syncViewport() Model {
	if !m.ready {
		return m
	}
	ctx := m.viewportRenderContext()
	rendered := make([]string, 0, len(m.records))
	for i := range m.records {
		out, cached := renderRecordCached(m.records[i], ctx)
		m.records[i] = cached
		rendered = append(rendered, out)
	}
	content := joinRenderedForViewport(m.records, rendered)
	m.contentLines = countContentLines(content)
	m.viewport.SetContent(content)
	return m
}

func joinRenderedForViewport(records []viewRecord, rendered []string) string {
	if len(rendered) == 0 {
		return ""
	}
	// Add a blank line between records for readability in the viewport, but never
	// insert a blank line *above* a system record. This keeps system notices
	// tightly attached to the line above (e.g. command echo → status lines),
	// while still allowing spacing after the system block before the next record.
	//
	// NOTE: blank lines increase rendered height and can trigger scrolling earlier.
	var b strings.Builder
	for i, renderedRec := range rendered {
		if i > 0 {
			sep := "\n\n"
			if i < len(records) && records[i].record.Kind == rec.KindSystem {
				sep = "\n"
			}
			b.WriteString(sep)
		}
		b.WriteString(renderedRec)
	}
	return b.String()
}
