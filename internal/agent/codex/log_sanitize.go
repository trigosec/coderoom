package codex

import "github.com/charmbracelet/x/ansi"

func sanitizeLogText(text string) string {
	return ansi.Strip(text)
}
