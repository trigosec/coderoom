package compose

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	"github.com/charmbracelet/x/ansi"
)

// promptWidth is computed (not a const) because we want the display width of
// the glyph as measured by the ANSI-aware width function.
var promptWidth = ansi.StringWidth("❯ ")

// View renders the input area.
//
// TODO: once charmbracelet/bubbles/v2 ships PR #822, the custom rendering
// below can be replaced with a targeted post-paste viewport reset via the
// public API. Requires migrating from bubbles v1 to v2 (different import
// path). See: https://charm.land/bubbles/v2/pull/822
func (m Model) View() string {
	// Don't rely on textarea's internal viewport rendering for multi-line input.
	// When pasting multiple logical lines, textarea's viewport can end up
	// scrolled such that only the last logical line is visible. Instead, render
	// the buffer ourselves using the same wrapping algorithm as textarea (see
	// wrap.go) and then pad/truncate to the configured height.
	h := m.input.Height()
	if h <= 0 {
		return ""
	}
	contentW := m.input.Width()
	lines := renderBufferLines(m.input.Value(), contentW)
	lines = decoratePromptLines(lines)
	lines = maybeOverlayCursor(lines, m.input, contentW)
	return fitLinesToHeight(lines, h, m.scrollOff)
}

func applyDecorations(input textarea.Model) textarea.Model {
	input.ShowLineNumbers = false
	input.SetPromptFunc(promptWidth, func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return "❯ "
		}
		return "  "
	})
	return input
}

func renderBufferLines(text string, contentW int) []string {
	if contentW <= 0 {
		contentW = 1
	}
	var out []string
	for line := range strings.SplitSeq(text, "\n") {
		wrapped := wrapTextareaLine([]rune(line), contentW)
		for _, wl := range wrapped {
			// textarea's wrap helper appends trailing spaces for cursor navigation
			// consistency; trim them for display so we don't render phantom blank
			// rows.
			out = append(out, strings.TrimRight(string(wl), " "))
		}
	}
	if len(out) == 0 {
		return []string{""}
	}
	// If the buffer does not end with a newline, drop trailing empty visual rows
	// introduced purely by wrapping/navigation padding.
	if !strings.HasSuffix(text, "\n") {
		for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
			out = out[:len(out)-1]
		}
		if len(out) == 0 {
			out = []string{""}
		}
	}
	return out
}

func decoratePromptLines(lines []string) []string {
	if len(lines) == 0 {
		return []string{"❯ "}
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if i == 0 {
			out[i] = "❯ " + line
		} else {
			out[i] = "  " + line
		}
	}
	return out
}

func fitLinesToHeight(lines []string, h int, scrollOff int) string {
	if h <= 0 {
		return ""
	}
	if len(lines) < h {
		padded := make([]string, h)
		copy(padded, lines)
		lines = padded
	} else if len(lines) > h {
		start := scrollOff
		if start < 0 {
			start = 0
		}
		maxStart := len(lines) - h
		if start > maxStart {
			start = maxStart
		}
		lines = lines[start : start+h]
	}
	return strings.Join(lines, "\n")
}

func maybeOverlayCursor(lines []string, input textarea.Model, contentW int) []string {
	if !input.Focused() {
		return lines
	}
	row, col, ok := cursorVisualPos(input, contentW)
	if !ok || row < 0 || row >= len(lines) || col < 0 {
		return lines
	}
	return overlayBlockCursor(lines, row, col)
}

func cursorVisualPos(input textarea.Model, contentW int) (row int, col int, ok bool) {
	if contentW <= 0 {
		contentW = 1
	}
	visualRow, visualCol := cursorVisualRowCol(input, contentW)
	return visualRow, promptWidth + visualCol, true
}

func overlayBlockCursor(lines []string, row int, col int) []string {
	out := make([]string, len(lines))
	copy(out, lines)

	r := []rune(out[row])
	if col > len(r) {
		pad := make([]rune, col-len(r))
		for i := range pad {
			pad[i] = ' '
		}
		r = append(r, pad...)
	}

	// Render a visible cursor without destroying the underlying character by
	// inverting the cell (SGR 7). This works well across terminals and keeps the
	// typed glyph readable.
	if col >= len(r) {
		r = append(r, ' ')
	}
	const invertOn = "\x1b[7m"
	const invertOff = "\x1b[0m"
	ch := r[col]
	out[row] = string(r[:col]) + invertOn + string(ch) + invertOff + string(r[col+1:])
	return out
}
