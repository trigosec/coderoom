package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// View renders the current model state as a string for Bubble Tea to display.
func (m Model) View() string {
	if !m.ready {
		return ""
	}
	left := strings.Repeat(" ", marginH)
	sepLabel := m.separatorLabel()
	sep := left + labeledSeparator(m.viewport.Width, sepLabel)

	var sb strings.Builder
	// Render the viewport area with stable height so the input/toolbox remain
	// anchored. We render line-by-line so we can apply the optional row-number
	// overlay in debug mode.
	m.writeViewport(&sb, left)
	sb.WriteString(sep + "\n")
	for line := range strings.SplitSeq(strings.TrimSuffix(m.compose.View(), "\n"), "\n") {
		sb.WriteString(left + line + "\n")
	}
	for _, line := range strings.Split(m.toolbox.View(), "\n") {
		sb.WriteString(left + line + "\n")
	}
	sb.WriteString(strings.Repeat("\n", marginV))
	// Avoid a trailing newline: when the rendered frame height matches the
	// terminal height, a final newline can scroll the terminal and make the
	// first row appear "missing".
	return strings.TrimRight(sb.String(), "\n")
}

func (m Model) separatorLabel() string {
	label := "compose"
	if m.focus == focusViewport {
		label = "history"
	}
	if !m.debug {
		return label
	}

	content := strings.Join(m.renderedRecords, "\n")
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

	return label +
		" y=" + strconv.Itoa(m.viewport.YOffset) +
		" h=" + strconv.Itoa(m.viewport.Height) +
		" rec=" + strconv.Itoa(len(m.records)) +
		" ln=" + strconv.Itoa(contentLines) +
		" first=" + first +
		" viewFirst=" + viewFirst +
		" viewWho=" + strconv.Itoa(viewWho) +
		" viewLn=" + strconv.Itoa(viewLines)
}

func (m Model) writeViewport(sb *strings.Builder, left string) {
	viewportView := strings.TrimSuffix(m.viewport.View(), "\n")
	viewportLines := []string{}
	if viewportView != "" {
		viewportLines = strings.Split(viewportView, "\n")
	}
	// Render line-by-line to support the optional row-number overlay (debug).
	for i := 0; i < m.viewport.Height; i++ {
		line := ""
		if i < len(viewportLines) {
			line = viewportLines[i]
		}
		if m.debugRowNums {
			line = strconv.Itoa(i+1) + ":" + line
		}
		sb.WriteString(left + line + "\n")
	}
}

func labeledSeparator(width int, label string) string {
	if width <= 0 {
		return ""
	}
	mid := " " + label + " "
	if len(mid) >= width {
		return strings.Repeat("─", width)
	}
	leftCount := (width - len(mid)) / 2
	rightCount := width - len(mid) - leftCount
	return strings.Repeat("─", leftCount) + mid + strings.Repeat("─", rightCount)
}
