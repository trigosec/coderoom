package history

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/agent"
)

// SetSize initialises or resizes the viewport.
func (m Model) SetSize(w, h int) Model {
	if !m.ready {
		m.viewport = viewport.New(w, h)
		m.ready = true
	} else {
		m.viewport.Width = w
		m.viewport.Height = h
	}
	return m.syncViewport()
}

// SetHeight adjusts the viewport height and re-syncs content.
func (m Model) SetHeight(h int) Model {
	m.viewport.Height = h
	return m.syncViewport()
}

// RebuildColors re-renders every record using the current color resolution.
func (m Model) RebuildColors() Model {
	for i, r := range m.records {
		m.renderedRecords[i] = renderRecord(r, m.viewport.Width, m.resolveColor)
	}
	return m.syncViewport()
}

// AppendRecord adds r to the record list, scrolling to bottom if already there.
func (m Model) AppendRecord(r Record) Model {
	wasAtBottom := m.viewport.AtBottom()
	m.records = append(m.records, r)
	m.renderedRecords = append(m.renderedRecords, renderRecord(r, m.viewport.Width, m.resolveColor))
	m = m.syncViewport()
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
	return m
}

// AppendSystemRecord appends a system-notice record with the given body.
func (m Model) AppendSystemRecord(body string) Model {
	return m.AppendRecord(Record{Kind: RecordKindSystem, Body: body})
}

// AppendUserInputRecord appends a user-input record with optional routing footer.
func (m Model) AppendUserInputRecord(body string, routing []string) Model {
	return m.AppendRecord(Record{Kind: RecordKindUserInput, Body: body, Routing: routing})
}

// AppendLogRecord appends a diagnostic log line from alias.
func (m Model) AppendLogRecord(alias, body string) Model {
	return m.AppendRecord(Record{Kind: RecordKindLog, Alias: alias, Body: body})
}

// HandleAgentMessage appends or extends a streaming record based on msg.
// Output+ModeFlush (turn-end) clears all open streams for alias.
// Reasoning+ModeFlush clears only the matching reasoning stream.
func (m Model) HandleAgentMessage(alias string, msg agent.Message) Model {
	switch msg.Content.(type) {
	case agent.Output:
		if msg.Mode == agent.ModeFlush {
			return m.clearStreamsForAlias(alias)
		}
		return m.appendOrExtend(alias, msg)
	case agent.Reasoning:
		if msg.Mode == agent.ModeFlush {
			delete(m.streaming, msg.StreamID)
			return m
		}
		return m.appendOrExtend(alias, msg)
	}
	return m
}

func (m Model) appendOrExtend(alias string, msg agent.Message) Model {
	wasAtBottom := m.viewport.AtBottom()
	if slot, ok := m.streaming[msg.StreamID]; ok {
		if accumulated, err := slot.msg.Accumulate(msg); err == nil {
			m.records[slot.recordIdx].Body = bodyFrom(accumulated)
			m.renderedRecords[slot.recordIdx] = renderRecord(m.records[slot.recordIdx], m.viewport.Width, m.resolveColor)
			m.streaming[msg.StreamID] = streamSlot{slot.recordIdx, accumulated}
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
	r := Record{Kind: recordKindFor(msg), Alias: alias, Body: bodyFrom(msg)}
	m.records = append(m.records, r)
	m.renderedRecords = append(m.renderedRecords, renderRecord(r, m.viewport.Width, m.resolveColor))
	m.streaming[msg.StreamID] = streamSlot{idx, msg}
	return m
}

func bodyFrom(msg agent.Message) string {
	switch c := msg.Content.(type) {
	case agent.Output:
		return c.Text
	case agent.Reasoning:
		return c.Text
	}
	return ""
}

func recordKindFor(msg agent.Message) RecordKind {
	if _, ok := msg.Content.(agent.Reasoning); ok {
		return RecordKindReasoning
	}
	return RecordKindAgentOutput
}

// MarkDeparted records alias as departed and re-renders its affected records.
func (m Model) MarkDeparted(alias string) Model {
	m.departed[alias] = true
	for i, r := range m.records {
		if r.Alias == alias || slices.Contains(r.Routing, alias) {
			m.renderedRecords[i] = renderRecord(r, m.viewport.Width, m.resolveColor)
		}
	}
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
		if _, ok := slot.msg.Content.(agent.Reasoning); ok {
			if m.records[slot.recordIdx].Alias == alias {
				return true
			}
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
func (m Model) YOffset() int { return m.viewport.YOffset }

func (m Model) syncViewport() Model {
	if !m.ready {
		return m
	}
	m.viewport.SetContent(joinRenderedForViewport(m.records, m.renderedRecords))
	return m
}

func joinRenderedForViewport(records []Record, rendered []string) string {
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
	for i, rec := range rendered {
		if i > 0 {
			sep := "\n\n"
			if i < len(records) && records[i].Kind == RecordKindSystem {
				sep = "\n"
			}
			b.WriteString(sep)
		}
		b.WriteString(rec)
	}
	return b.String()
}
