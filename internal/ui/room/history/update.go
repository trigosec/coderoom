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

// AppendRecord adds r to the record list, scrolling to bottom if already there.
func (m Model) AppendRecord(r rec.Record) Model {
	wasAtBottom := m.viewport.AtBottom()
	ctx := m.viewportRenderContext()
	_, cached := r.RenderCached(ctx)
	m.records = append(m.records, cached)
	m = m.syncViewport()
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
	return m
}

// AppendSystemRecord appends a system-notice record with the given body.
func (m Model) AppendSystemRecord(body string) Model {
	return m.AppendRecord(rec.Record{Kind: rec.KindSystem, Text: body})
}

// AppendUserInputRecord appends a user-input record with optional routing footer.
func (m Model) AppendUserInputRecord(body string, routing []string) Model {
	return m.AppendRecord(rec.Record{Kind: rec.KindUserInput, Text: body, Routing: routing})
}

// AppendLogRecord appends a diagnostic log line from alias.
func (m Model) AppendLogRecord(alias, body string) Model {
	return m.AppendRecord(rec.Record{Kind: rec.KindLog, Alias: alias, Text: body})
}

// HandleAgentMessage appends or extends a streaming record based on msg.
// Output+ModeFlush seals the matching output stream.
// Reasoning+ModeFlush clears only the matching reasoning stream.
// Command+ModeFlush seals the stream; the exit code was accumulated via the
// preceding Command+ModeStream from item/completed.
func (m Model) HandleAgentMessage(alias string, msg agent.Message) Model {
	switch msg.Content.(type) {
	case agent.Output:
		if msg.Mode == agent.ModeFlush {
			return m.sealOutputStream(msg.StreamID)
		}
		return m.appendOrExtend(alias, msg)
	case agent.Reasoning:
		if msg.Mode == agent.ModeFlush {
			delete(m.streaming, msg.StreamID)
			return m
		}
		return m.appendOrExtend(alias, msg)
	case agent.Command:
		if msg.Mode == agent.ModeFlush {
			return m.sealCommandStream(msg.StreamID)
		}
		return m.appendOrExtend(alias, msg)
	case agent.FileChangeSet:
		if msg.Mode == agent.ModeFlush {
			return m.sealFileChangeStream(msg.StreamID)
		}
		return m.appendOrExtend(alias, msg)
	}
	return m
}

func (m Model) appendOrExtend(alias string, msg agent.Message) Model {
	wasAtBottom := m.viewport.AtBottom()
	if slot, ok := m.streaming[msg.StreamID]; ok {
		if updated, err := m.records[slot.recordIdx].Accumulate(msg); err == nil {
			_, cached := updated.RenderCached(m.viewportRenderContext())
			m.records[slot.recordIdx] = cached
		} else {
			// Content-type mismatch on a live stream: preserve the existing record
			// and open a fresh one rather than wiping the accumulated body.
			m = m.openRecord(alias, msg)
		}
	} else {
		m = m.openRecord(alias, msg)
	}
	m = m.syncViewport()
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
	return m
}

func (m Model) openRecord(alias string, msg agent.Message) Model {
	idx := len(m.records)
	r := rec.NewAgent(alias, msg)
	_, cached := r.RenderCached(m.viewportRenderContext())
	m.records = append(m.records, cached)
	m.streaming[msg.StreamID] = streamSlot{recordIdx: idx}
	return m
}

func (m Model) sealCommandStream(streamID agent.StreamID) Model {
	return m.sealStream(streamID)
}

func (m Model) sealOutputStream(streamID agent.StreamID) Model {
	return m.sealStream(streamID)
}

func (m Model) sealFileChangeStream(streamID agent.StreamID) Model {
	return m.sealStream(streamID)
}

func (m Model) sealStream(streamID agent.StreamID) Model {
	slot, ok := m.streaming[streamID]
	if !ok {
		return m
	}
	wasAtBottom := m.viewport.AtBottom()
	_, cached := m.records[slot.recordIdx].RenderCached(m.viewportRenderContext())
	m.records[slot.recordIdx] = cached
	delete(m.streaming, streamID)
	m = m.syncViewport()
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
	return m
}

// MarkDeparted records alias as departed and re-renders its affected records.
func (m Model) MarkDeparted(alias string) Model {
	m.departed[alias] = true
	m.colorVersion++
	return m.syncViewport()
}

// ClearStreaming removes all open streams for alias (e.g., on agent departure).
func (m Model) ClearStreaming(alias string) Model {
	return m.clearStreamsForAlias(alias)
}

func (m Model) clearStreamsForAlias(alias string) Model {
	for streamID, slot := range m.streaming {
		if m.records[slot.recordIdx].Alias == alias {
			delete(m.streaming, streamID)
		}
	}
	return m
}

// IsReasoningStreaming reports whether alias has an open reasoning stream.
func (m Model) IsReasoningStreaming(alias string) bool {
	for _, slot := range m.streaming {
		r := m.records[slot.recordIdx]
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
		out, cached := m.records[i].RenderCached(ctx)
		m.records[i] = cached
		rendered = append(rendered, out)
	}
	content := joinRenderedForViewport(m.records, rendered)
	m.contentLines = countContentLines(content)
	m.viewport.SetContent(content)
	return m
}

func joinRenderedForViewport(records []rec.Record, rendered []string) string {
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
			if i < len(records) && records[i].Kind == rec.KindSystem {
				sep = "\n"
			}
			b.WriteString(sep)
		}
		b.WriteString(renderedRec)
	}
	return b.String()
}
