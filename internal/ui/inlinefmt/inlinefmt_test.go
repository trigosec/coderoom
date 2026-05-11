package inlinefmt

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func withANSIProfile(t *testing.T, fn func()) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	fn()
}

func TestFormat_keepsDelimitersVisible(t *testing.T) {
	withANSIProfile(t, func() {
		in := `Here is **bold** and *italic* and ` + "`code`."
		out := Format(in, lipgloss.NewStyle().Foreground(lipgloss.Color("2")))
		if got := ansi.Strip(out); got != in {
			t.Fatalf("expected stripped output to equal input\nin:  %q\ngot: %q", in, got)
		}
		if out == in {
			t.Fatal("expected ANSI styling to be applied, got identical output")
		}
	})
}

func TestFormat_unmatchedDelimitersDegradeGracefully(t *testing.T) {
	withANSIProfile(t, func() {
		in := "Here is **bold"
		out := Format(in, lipgloss.NewStyle().Foreground(lipgloss.Color("2")))
		if out != in {
			t.Fatalf("expected unmatched opener to render as plain text\ngot: %q", out)
		}
	})
}

func TestFormat_unmatchedMultiCharOpenerDoesNotTriggerInnerMatch(t *testing.T) {
	// If "**" cannot be closed, we must not allow the second "*" to start an
	// italic span.
	withANSIProfile(t, func() {
		in := "**bold*"
		out := Format(in, lipgloss.NewStyle().Foreground(lipgloss.Color("2")))
		if out != in {
			t.Fatalf("expected unmatched multi-char opener to render as plain text\ngot: %q", out)
		}
		if strings.Contains(out, "\x1b[") {
			t.Fatalf("expected no ANSI styling when no spans match, got: %q", out)
		}
	})
}

func TestFormat_asciiSingleQuoteBoundaryRule(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

	withANSIProfile(t, func() {
		in := "it's fine"
		out := Format(in, style)
		if out != in {
			t.Fatalf("expected apostrophe to be treated as plain text\ngot: %q", out)
		}

		quoted := "the 'quick' fox"
		out = Format(quoted, style)
		if ansi.Strip(out) != quoted {
			t.Fatalf("expected stripped output to equal input\ngot: %q", ansi.Strip(out))
		}
		if out == quoted {
			t.Fatal("expected quote span to be styled")
		}
	})
}

func TestFormat_quotesAndAdjacentSpans(t *testing.T) {
	withANSIProfile(t, func() {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
		cases := []string{
			`He said "hello".`,
			"She said “hello”.",
			"She said ‘hello’.",
			"She said «hello».",
			"She said ‹hello›.",
			"`code` **bold**",
		}
		for _, in := range cases {
			out := Format(in, style)
			if got := ansi.Strip(out); got != in {
				t.Fatalf("expected stripped output to equal input\nin:  %q\ngot: %q", in, got)
			}
			if out == in {
				t.Fatalf("expected styling to be applied for %q", in)
			}
		}
	})
}

func TestFormat_doesNotStyleEmptySpans(t *testing.T) {
	withANSIProfile(t, func() {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
		in := `"" '' **** ` + "``"
		out := Format(in, style)
		if out != in {
			t.Fatalf("expected empty spans to remain plain text\ngot: %q", out)
		}
	})
}

func TestFormat_boldNotTwoItalics(t *testing.T) {
	withANSIProfile(t, func() {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
		in := "**text**"
		out := Format(in, style)
		if got := ansi.Strip(out); got != in {
			t.Fatalf("expected stripped output to equal input\ngot: %q", got)
		}
		if !strings.Contains(out, "\x1b[1m") && !strings.Contains(out, "\x1b[1;") {
			t.Fatalf("expected bold SGR in output, got: %q", out)
		}
		if strings.Contains(out, "\x1b[3m") {
			t.Fatalf("expected no italic SGR in bold output, got: %q", out)
		}
	})
}
