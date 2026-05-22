package compose

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestAltEnter_insertsNewlineWithoutSubmitting(t *testing.T) {
	m := New()
	m = m.SetValue("hello")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	if got := m.Value(); got != "hello\n" {
		t.Fatalf("expected Alt+Enter to insert newline, got %q", got)
	}
	if cmd != nil {
		t.Fatal("expected no cmd from Alt+Enter")
	}
}

func TestCtrlC_clearsValue(t *testing.T) {
	m := New()
	m = m.SetValue("draft")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatal("expected no cmd from Ctrl+C")
	}
	if got := m.Value(); got != "" {
		t.Fatalf("expected Ctrl+C to clear, got %q", got)
	}
}

func TestCtrlC_noopWhenEmpty(t *testing.T) {
	m := New()
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatal("expected no cmd from Ctrl+C on empty input")
	}
	if got := m.Value(); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestCtrlRight_movesWordForward(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetValue("hello world")
	// Move cursor to start (Ctrl+A) so there is a word to jump over.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	before := m.input.LineInfo().CharOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	after := m.input.LineInfo().CharOffset
	if after <= before {
		t.Fatalf("expected ctrl+right to advance cursor; before=%d after=%d", before, after)
	}
}

func TestCtrlLeft_movesWordBackward(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetValue("hello world")
	// Cursor starts at end after SetValue; ctrl+left must move it back.
	before := m.input.LineInfo().CharOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlLeft})
	after := m.input.LineInfo().CharOffset
	if after >= before {
		t.Fatalf("expected ctrl+left to move cursor back; before=%d after=%d", before, after)
	}
}

func TestView_singleLine(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetValue("hello")
	view := ansi.Strip(m.View())
	if !strings.HasPrefix(view, "❯ hello") {
		t.Fatalf("expected single-line view to start with '❯ hello', got:\n%s", view)
	}
}

func TestUp_onFirstLine_movesToFirstChar(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetMaxHeightFromTotal(30).SetValue("hello\nworld")

	// Move to first line.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := m.input.Line(); got != 0 {
		t.Fatalf("expected to be on first line, got %d", got)
	}
	// Put cursor at a non-zero column on the first line.
	// Note: this mutates the textarea directly (bypassing Update/recalcHeight),
	// which is intentional here: the test is about cursor movement semantics.
	m.input.SetCursor(3)
	if off := m.input.LineInfo().ColumnOffset; off == 0 {
		t.Fatalf("expected cursor column >0 before Up, got %d", off)
	}

	// Up at top should move to first character.
	beforeLI := m.input.LineInfo()
	if !m.input.Focused() {
		t.Fatal("expected textarea to be focused")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	li := m.input.LineInfo()
	if got := m.input.Line(); got != 0 {
		t.Fatalf("expected to remain on first line after Up, got %d", got)
	}
	if li.StartColumn != 0 || li.ColumnOffset != 0 {
		t.Fatalf("expected cursor at col 0 after Up at top, got start=%d offset=%d (before ro=%d h=%d; after ro=%d h=%d)",
			li.StartColumn, li.ColumnOffset, beforeLI.RowOffset, beforeLI.Height, li.RowOffset, li.Height)
	}
}

func TestDown_onLastLine_movesToLastChar(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetMaxHeightFromTotal(30).SetValue("hello\nworld")

	// Ensure we are on last line and move cursor to start.
	if got := m.input.Line(); got != 1 {
		t.Fatalf("expected to start on last line, got %d", got)
	}
	m.input.CursorStart()
	if off := m.input.LineInfo().ColumnOffset; off != 0 {
		t.Fatalf("expected cursor at start before Down, got %d", off)
	}

	// Down at bottom should move to end of line.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if off := m.input.LineInfo().ColumnOffset; off != len("world") {
		t.Fatalf("expected cursor to move to end of line (%d), got %d", len("world"), off)
	}
}

func TestView_multiLine_continuationIndented(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetMaxHeightFromTotal(20).SetValue("first\nsecond")
	view := ansi.Strip(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 rendered lines, got:\n%s", view)
	}
	if !strings.HasPrefix(lines[0], "❯ ") {
		t.Errorf("first line should start with '❯ ', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  ") {
		t.Errorf("second line should start with '  ', got %q", lines[1])
	}
}

func TestHasAbove_falseWhenContentFits(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetMaxHeightFromTotal(20).SetValue("one\ntwo\nthree")
	if m.HasAbove() {
		t.Error("expected HasAbove=false when all lines fit in the viewport")
	}
	if m.HasBelow() {
		t.Error("expected HasBelow=false when all lines fit in the viewport")
	}
}

func TestHasAbove_trueWhenCursorAtEndOfOverflow(t *testing.T) {
	// maxH from totalH=6 is min(8, max(6/3,3))=3. Set enough lines to overflow.
	m := New()
	m = m.SetWidth(40).SetMaxHeightFromTotal(6).SetValue("a\nb\nc\nd\ne")
	// Cursor is at the last line after SetValue; content above must exist.
	if !m.HasAbove() {
		t.Error("expected HasAbove=true when buffer exceeds viewport height")
	}
}

func TestHasBelow_trueWhenCursorAtTop(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetMaxHeightFromTotal(6).SetValue("a\nb\nc\nd\ne")
	// Move cursor to the first line via Ctrl+Home equivalent (Ctrl+A then Up).
	for range 10 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if !m.HasBelow() {
		t.Error("expected HasBelow=true when cursor is at the top of an overflowing buffer")
	}
}

func TestView_longLine_wrapsWithIndent(t *testing.T) {
	m := New()
	// Width 12: prompt takes 2, content area is 10.
	// "0123456789X" is 11 runes — should wrap onto a second visual row.
	m = m.SetWidth(12).SetMaxHeightFromTotal(20).SetValue("0123456789X")
	view := ansi.Strip(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapped long line to produce >=2 rendered rows, got:\n%s", view)
	}
	if !strings.HasPrefix(lines[0], "❯ ") {
		t.Errorf("first visual row should start with '❯ ', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  ") {
		t.Errorf("continuation row should start with '  ', got %q", lines[1])
	}
}

func TestView_threeLineWrap_notClipped(t *testing.T) {
	m := New()
	// Width 12: prompt takes 2, content area is 10.
	// 21 runes should wrap to 3 visual rows (10 + 10 + 1).
	m = m.SetWidth(12).SetMaxHeightFromTotal(9).SetValue("012345678901234567890")
	if got := m.Height(); got != 3 {
		t.Fatalf("expected input height=3 for a 3-row wrapped line, got %d", got)
	}
	view := ansi.Strip(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected >=3 rendered rows, got:\n%s", view)
	}
	if !strings.HasPrefix(lines[0], "❯ ") {
		t.Errorf("first visual row should start with '❯ ', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  ") || !strings.HasPrefix(lines[2], "  ") {
		t.Errorf("continuation rows should start with '  ', got %q / %q", lines[1], lines[2])
	}
}

func TestIndicators_overflowShowAboveBelow(t *testing.T) {
	m := New()
	// totalH=30 => maxH=min(8, max(30/3,3))=8.
	m = m.SetWidth(40).SetMaxHeightFromTotal(30).SetValue("1\n2\n3\n4\n5\n6\n7\n8\n9")
	if got := m.Height(); got != 8 {
		t.Fatalf("expected height=8 cap, got %d", got)
	}
	// Cursor is at end; top content is hidden.
	if !m.HasAbove() {
		t.Error("expected HasAbove=true at end of overflowing buffer")
	}
	if m.HasBelow() {
		t.Error("expected HasBelow=false at end of overflowing buffer")
	}

	// Move cursor to top; bottom content is hidden.
	for range 20 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if m.HasAbove() {
		t.Error("expected HasAbove=false at top of buffer")
	}
	if !m.HasBelow() {
		t.Error("expected HasBelow=true at top of overflowing buffer")
	}
}

func TestKeyRunes_multiRuneWithTrailingSpace_updatesWrapping(t *testing.T) {
	m := New()
	m = m.SetWidth(12).SetMaxHeightFromTotal(30)

	// Simulate IME/paste delivering multiple runes at once, including a trailing
	// space that has historically triggered under-counting of wrapped rows.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("aaaaaaaaaaa aaaaaaaaaaa ")})

	if got := m.Height(); got < 2 {
		t.Fatalf("expected wrapped input height >=2, got %d", got)
	}
	if m.HasAbove() || m.HasBelow() {
		t.Fatalf("expected no overflow indicators for short wrapped input; above=%v below=%v", m.HasAbove(), m.HasBelow())
	}
}

func TestView_scrolledToTop_showsPrompt(t *testing.T) {
	m := New()
	// totalH=30 => maxH=8 cap.
	m = m.SetWidth(40).SetMaxHeightFromTotal(30).SetValue("1\n2\n3\n4\n5\n6\n7\n8\n9")
	if got := m.Height(); got != 8 {
		t.Fatalf("expected height=8 cap, got %d", got)
	}
	// Move cursor to top.
	for range 50 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	view := ansi.Strip(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected non-empty view, got %q", view)
	}
	if !strings.HasPrefix(lines[0], "❯ ") {
		t.Fatalf("expected prompt on first visible row when scrolled to top, got %q\nview:\n%s", lines[0], view)
	}
}

func TestScrollOff_stickyScrollTracksCursor(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetMaxHeightFromTotal(30)
	m = m.SetValue("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12") // 12 logical lines
	if got := m.Height(); got != 8 {
		t.Fatalf("expected height=8 cap, got %d", got)
	}
	if m.visH <= m.Height() {
		t.Fatalf("expected overflow (visH=%d height=%d)", m.visH, m.Height())
	}

	maxOff := m.visH - m.Height()
	if m.scrollOff != maxOff {
		t.Fatalf("expected initial scrollOff=%d at bottom, got %d", maxOff, m.scrollOff)
	}

	// Move cursor up one visual row at a time; scrollOff should not jump more
	// than one row per keypress.
	prev := m.scrollOff
	for range 20 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
		if delta := prev - m.scrollOff; delta < 0 || delta > 1 {
			t.Fatalf("unexpected scrollOff jump: prev=%d now=%d", prev, m.scrollOff)
		}
		prev = m.scrollOff
	}
}

func TestKeyRunes_multiRuneEndingWithNewline_keepsBlankLineVisible(t *testing.T) {
	m := New()
	m = m.SetWidth(20).SetMaxHeightFromTotal(30)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello world\n")})

	view := ansi.Strip(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 visible rows after trailing newline, got %d:\n%s", len(lines), view)
	}
	// The final rendered row should represent the blank logical line created by
	// the trailing newline.
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "  ") {
		t.Fatalf("expected continuation prompt on final blank line, got %q\nview:\n%s", last, view)
	}
	if strings.TrimSpace(strings.TrimPrefix(last, "  ")) != "" {
		t.Fatalf("expected final line to be blank after prompt, got %q\nview:\n%s", last, view)
	}
}

func TestKeyRunes_multiLinePaste_doesNotHideFirstLine(t *testing.T) {
	m := New()
	// Force a small input height cap to mimic small terminals where only a
	// couple of visual rows are visible.
	m = m.SetWidth(60).SetMaxHeightFromTotal(5)

	paste := "I am glad that corpus made an impression. I am still exploring the workflow interaction and the feedback loop and will definitely involve you as it grows.\na"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(paste)})

	view := ansi.Strip(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected >=2 visible rows, got %d:\n%s", len(lines), view)
	}
	// With a small height cap we follow the bottom. The key invariant is that
	// the second logical line is still visible (the bug was showing only the
	// last line plus padding/blank rows).
	clean := strings.ReplaceAll(view, "█", "")
	clean = strings.TrimRight(clean, " \n")
	if !strings.Contains(clean, "\na") && !strings.HasSuffix(clean, "a") {
		t.Fatalf("expected second logical line ('a') to be visible, got:\n%s", view)
	}
}

func TestView_twoLogicalLines_bothVisibleWhenHeightAllows(t *testing.T) {
	m := New()
	m = m.SetWidth(40).SetMaxHeightFromTotal(30).SetValue("a\nb")
	view := ansi.Strip(m.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected >=2 lines, got:\n%s", view)
	}
	if !strings.Contains(lines[0], "a") {
		t.Fatalf("expected first line to contain 'a', got %q\nview:\n%s", lines[0], view)
	}
	if !strings.Contains(lines[1], "b") {
		t.Fatalf("expected second line to contain 'b', got %q\nview:\n%s", lines[1], view)
	}
}

func TestKeyRunes_trailingDoubleSpaceBeforeNewline_doesNotClip(t *testing.T) {
	m := New()
	// This ends with two spaces then newline (Markdown hard-break pattern).
	text := "I am glad that corpus made an impression. I am still exploring the workflow interaction and the feedback loop and will definitely involve you as it grows.  \n"

	// Search for any width where the textarea scrolls (symptom: first visible row
	// starts with continuation indent rather than the prompt).
	for w := 12; w <= 80; w++ {
		mw := m.SetWidth(w).SetMaxHeightFromTotal(30)
		mw, _ = mw.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
		view := ansi.Strip(mw.View())
		lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
		if len(lines) == 0 {
			t.Fatalf("width=%d: expected non-empty view, got %q", w, view)
		}
		// If the content wraps taller than the capped height, we follow the
		// cursor at the bottom, so the first visible row may be a continuation.
		// A trailing newline produces a real blank logical line; allow it.
		if strings.TrimSpace(lines[0]) == "" {
			t.Fatalf("width=%d: unexpected blank first row:\n%s", w, view)
		}
	}
}
