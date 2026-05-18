package room

import (
	"strings"
)

// View renders the room as:
//
//	history viewport
//	separator line
//	composer input
//
// It does not render outer chrome (toolbox, bottom padding, margins).
func (m Model) View() string {
	if !m.history.Ready() {
		return ""
	}
	sep := labeledSeparator(m.history.Width(), m.separatorLabel())

	var sb strings.Builder
	for line := range strings.SplitSeq(m.history.View(), "\n") {
		sb.WriteString(line + "\n")
	}
	sb.WriteString(sep + "\n")
	if m.input.kind == inputApproval {
		for line := range strings.SplitSeq(m.input.approval.View(), "\n") {
			sb.WriteString(line + "\n")
		}
	} else {
		for line := range strings.SplitSeq(strings.TrimSuffix(m.input.compose.View(), "\n"), "\n") {
			sb.WriteString(line + "\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (m Model) separatorLabel() string {
	label := "compose"
	if m.focus == focusHistory {
		label = "history"
	}
	if m.input.kind == inputApproval && m.focus != focusHistory {
		label = "approval"
	}
	if !m.debug {
		return label
	}
	return label + " " + m.history.DebugLabel()
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
