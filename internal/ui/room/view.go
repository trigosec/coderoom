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
	sep := labeledSeparator(m.history.Width(), m.separatorLabel(), m.composeIndicators())

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
	sb.WriteString(m.bottomSeparator() + "\n")
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

func (m Model) bottomSeparator() string {
	w := m.history.Width()
	if w <= 0 {
		return ""
	}
	suffix := ""
	if m.input.kind == inputCompose && m.input.compose.HasBelow() {
		suffix = "▼"
	}
	suffixW := len([]rune(suffix))
	return strings.Repeat("─", max(0, w-suffixW)) + suffix
}

func (m Model) composeIndicators() string {
	if m.input.kind == inputCompose && m.input.compose.HasAbove() {
		return "▲"
	}
	return ""
}

func labeledSeparator(width int, label, suffix string) string {
	if width <= 0 {
		return ""
	}
	suffixW := len([]rune(suffix)) // ▲▼ are single-display-width chars
	mid := " " + label + " "
	avail := width - suffixW
	if len(mid) >= avail {
		return strings.Repeat("─", max(avail, 0)) + suffix
	}
	leftCount := (avail - len(mid)) / 2
	rightCount := avail - len(mid) - leftCount
	return strings.Repeat("─", leftCount) + mid + strings.Repeat("─", rightCount) + suffix
}
