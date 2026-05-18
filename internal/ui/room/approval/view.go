package approval

import (
	"strings"

	"github.com/trigosec/coderoom/internal/agent"
)

// View renders the approval prompt and options.
func (m Model) View() string {
	if !m.Active() {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.ask)
	b.WriteString("\n\n")
	for i, opt := range m.options {
		prefix := "  "
		if i == m.selected {
			prefix = "> "
		}
		b.WriteString(prefix)
		b.WriteString(formatOption(opt))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatOption(opt agent.ApprovalOption) string {
	switch opt {
	case agent.OptionAccept:
		return "accept"
	case agent.OptionAcceptForSession:
		return "accept for session"
	case agent.OptionDecline:
		return "decline"
	case agent.OptionCancel:
		return "cancel"
	default:
		return string(opt)
	}
}
