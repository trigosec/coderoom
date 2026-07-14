package history

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderCursorLine_preservesStyledContent(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("hello")

	got := renderCursorLine(styled, 1, 10)

	if !strings.Contains(got, "\x1b[32m") {
		t.Fatalf("expected cursor line to preserve existing color styling, got %q", got)
	}
	if !strings.Contains(got, "\x1b[7m") {
		t.Fatalf("expected cursor line to add reverse-video cursor, got %q", got)
	}
}

func TestRenderCursorLine_usesDisplayWidthForWideGraphemes(t *testing.T) {
	got := renderCursorLine("a界b", 1, 10)

	if !strings.Contains(got, "a\x1b[7m界\x1b[27mb") {
		t.Fatalf("expected cursor to highlight the wide grapheme at display column 1, got %q", got)
	}
}

func TestRenderCursorLine_endOfFullWidthLineDoesNotOverflow(t *testing.T) {
	got := renderCursorLine("hello", 5, 5)

	if strings.Count(got, "\n") != 0 {
		t.Fatalf("expected full-width EOL cursor rendering to stay on one line, got %q", got)
	}
	if ansi.StringWidth(got) != 5 {
		t.Fatalf("expected full-width EOL cursor rendering width=5, got %d in %q", ansi.StringWidth(got), got)
	}
	if !strings.Contains(got, "\x1b[7m") {
		t.Fatalf("expected EOL cursor rendering to stay visible, got %q", got)
	}
}
