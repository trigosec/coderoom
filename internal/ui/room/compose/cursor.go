package compose

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
)

// cursorVisualRowCol returns the cursor position in visual (soft-wrapped)
// coordinates relative to the top of the full rendered buffer (not the
// viewport). Column is relative to the start of the content area.
func cursorVisualRowCol(input textarea.Model, contentW int) (row int, col int) {
	if contentW <= 0 {
		contentW = 1
	}
	cursorLogicalLine := input.Line()
	li := input.LineInfo()

	visual := 0
	idx := 0
	for line := range strings.SplitSeq(input.Value(), "\n") {
		if idx >= cursorLogicalLine {
			break
		}
		visual += len(wrapTextareaLine([]rune(line), contentW))
		idx++
	}

	return visual + li.RowOffset, li.ColumnOffset
}
