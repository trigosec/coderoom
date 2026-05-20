package history

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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

// HandleDelta appends or extends the streaming record for alias.
func (m Model) HandleDelta(alias, text string) Model {
	wasAtBottom := m.viewport.AtBottom()
	if idx, ok := m.streaming[alias]; ok {
		m.records[idx].Body += text
		m.renderedRecords[idx] = renderRecord(m.records[idx], m.viewport.Width, m.resolveColor)
	} else {
		idx = len(m.records)
		m.streaming[alias] = idx
		r := Record{Kind: RecordKindAgentOutput, Alias: alias, Body: text}
		m.records = append(m.records, r)
		m.renderedRecords = append(m.renderedRecords, renderRecord(r, m.viewport.Width, m.resolveColor))
	}
	m = m.syncViewport()
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
	return m
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

// HandleReasoningDelta appends or extends the reasoning streaming record for alias.
func (m Model) HandleReasoningDelta(alias, text string) Model {
	wasAtBottom := m.viewport.AtBottom()
	if idx, ok := m.reasoningStreaming[alias]; ok {
		m.records[idx].Body += text
		m.renderedRecords[idx] = renderRecord(m.records[idx], m.viewport.Width, m.resolveColor)
	} else {
		idx = len(m.records)
		m.reasoningStreaming[alias] = idx
		r := Record{Kind: RecordKindReasoning, Alias: alias, Body: text}
		m.records = append(m.records, r)
		m.renderedRecords = append(m.renderedRecords, renderRecord(r, m.viewport.Width, m.resolveColor))
	}
	m = m.syncViewport()
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
	return m
}

// ClearStreaming removes alias from both active-streaming sets.
func (m Model) ClearStreaming(alias string) Model {
	delete(m.streaming, alias)
	delete(m.reasoningStreaming, alias)
	return m
}

// ClearReasoningStreaming seals the open reasoning record for alias without
// affecting the output streaming slot or participant status.
func (m Model) ClearReasoningStreaming(alias string) Model {
	delete(m.reasoningStreaming, alias)
	return m
}

// IsReasoningStreaming reports whether alias currently has an open reasoning record.
func (m Model) IsReasoningStreaming(alias string) bool {
	_, ok := m.reasoningStreaming[alias]
	return ok
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
