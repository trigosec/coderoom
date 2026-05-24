package history

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	rec "github.com/trigosec/coderoom/internal/ui/room/history/record"
)

// View renders the viewport with an optional row-number overlay. The output is
// always exactly viewport.Height lines joined by newlines, with no trailing
// newline, so the outer layout gets a stable height.
func (m Model) View() string {
	if !m.ready {
		return ""
	}
	viewportView := strings.TrimSuffix(m.viewport.View(), "\n")
	var viewportLines []string
	if viewportView != "" {
		viewportLines = strings.Split(viewportView, "\n")
	}
	lines := make([]string, m.viewport.Height)
	for i := range lines {
		if i < len(viewportLines) {
			lines[i] = viewportLines[i]
		}
		if m.debugRowNums {
			lines[i] = strconv.Itoa(i+1) + ":" + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

// RenderedContent returns the raw rendered records joined by newlines; useful
// for checking all history content regardless of scroll position.
func (m Model) RenderedContent() string {
	ctx := m.viewportRenderContext()
	parts := make([]string, 0, len(m.records))
	for i := range m.records {
		out, cached := m.records[i].RenderCached(ctx)
		m.records[i] = cached
		parts = append(parts, out)
	}
	return strings.Join(parts, "\n")
}

// PlainText returns the ANSI-stripped, double-newline-joined rendered records
// for transcript export.
func (m Model) PlainText() string {
	parts := make([]string, 0, len(m.records))
	ctx := rec.RenderContext{
		Key:           rec.RenderKey{Mode: rec.RenderTranscript, ColorVersion: m.colorVersion},
		ColorForAlias: m.resolveColor,
	}
	for _, r := range m.records {
		parts = append(parts, r.Render(ctx))
	}
	return strings.Join(parts, "\n\n")
}

// DebugLabel returns a compact summary string for the separator label.
func (m Model) DebugLabel() string {
	content := m.RenderedContent()
	contentLines := 0
	first := ""
	if content != "" {
		contentLines = strings.Count(content, "\n") + 1
		first = content
		if i := strings.IndexByte(first, '\n'); i >= 0 {
			first = first[:i]
		}
		first = strings.TrimSpace(ansi.Strip(first))
		if len(first) > 24 {
			first = first[:24]
		}
	}

	viewContent := strings.TrimSuffix(m.viewport.View(), "\n")
	viewFirst := ""
	viewWho := 0
	viewLines := 0
	if viewContent != "" {
		viewLines = strings.Count(viewContent, "\n") + 1
		viewWho = strings.Count(ansi.Strip(viewContent), "❯ /who")
		viewFirst = viewContent
		if i := strings.IndexByte(viewFirst, '\n'); i >= 0 {
			viewFirst = viewFirst[:i]
		}
		viewFirst = strings.TrimSpace(ansi.Strip(viewFirst))
		if len(viewFirst) > 24 {
			viewFirst = viewFirst[:24]
		}
	}

	return fmt.Sprintf("y=%d h=%d rec=%d ln=%d first=%s viewFirst=%s viewWho=%d viewLn=%d",
		m.viewport.YOffset, m.viewport.Height,
		len(m.records), contentLines,
		first, viewFirst,
		viewWho, viewLines)
}

// DebugSummary returns a multi-line string summarising the viewport top for
// the /debugview command.
func (m Model) DebugSummary() string {
	view := ansi.Strip(strings.TrimSuffix(m.viewport.View(), "\n"))
	var lines []string
	if view != "" {
		lines = strings.Split(view, "\n")
	}
	if len(lines) > 8 {
		lines = lines[:8]
	}
	parts := []string{
		fmt.Sprintf("  y=%d h=%d rec=%d", m.viewport.YOffset, m.viewport.Height, len(m.records)),
		"  viewTop:",
	}
	for _, line := range lines {
		parts = append(parts, "    "+line)
	}
	return strings.Join(parts, "\n")
}
