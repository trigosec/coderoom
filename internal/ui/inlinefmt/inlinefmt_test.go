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

func TestFormat_omitsBoldAndCodeDelimiters(t *testing.T) {
	withANSIProfile(t, func() {
		in := `Here is **bold** and *italic* and ` + "`code`."
		out := Format(in, lipgloss.NewStyle().Foreground(lipgloss.Color("2")))
		want := "Here is bold and *italic* and code."
		if got := ansi.Strip(out); got != want {
			t.Fatalf("expected stripped output to equal desired output\nwant: %q\ngot:  %q", want, got)
		}
		if out == want {
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

func TestFormat_tripleBackticksAreTreatedAsCode(t *testing.T) {
	withANSIProfile(t, func() {
		in := "```text```"
		out := Format(in, lipgloss.NewStyle().Foreground(lipgloss.Color("2")))
		if got := ansi.Strip(out); got != "text" {
			t.Fatalf("expected stripped output to omit triple backtick delimiters\ngot: %q", got)
		}
		if out == "text" {
			t.Fatal("expected ANSI styling to be applied, got identical output")
		}
	})
}

func TestFormat_tripleBackticksTakePriorityOverSingleBackticks(t *testing.T) {
	// If single-backtick spans were tried before triple-backtick spans at the
	// same opener position, input like ```text``` could be misparsed (e.g. by
	// consuming only part of the delimiter run and leaving stray backticks).
	withANSIProfile(t, func() {
		in := "```text``` then"
		out := Format(in, lipgloss.NewStyle().Foreground(lipgloss.Color("2")))
		if got := ansi.Strip(out); got != "text then" {
			t.Fatalf("expected stripped output to parse as a single triple-backtick code span\nwant: %q\ngot:  %q", "text then", got)
		}
		if strings.Contains(ansi.Strip(out), "`") {
			t.Fatalf("expected no backticks to remain after rendering triple-backtick span\ngot: %q", ansi.Strip(out))
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
		cases := []struct {
			in   string
			want string
		}{
			{in: `He said "hello".`, want: `He said "hello".`},
			{in: "She said “hello”.", want: "She said “hello”."},
			{in: "She said ‘hello’.", want: "She said ‘hello’."},
			{in: "She said «hello».", want: "She said «hello»."},
			{in: "She said ‹hello›.", want: "She said ‹hello›."},
			{in: "`code` **bold**", want: "code bold"},
		}
		for _, tc := range cases {
			out := Format(tc.in, style)
			if got := ansi.Strip(out); got != tc.want {
				t.Fatalf("expected stripped output to equal desired output\nin:   %q\nwant: %q\ngot:  %q", tc.in, tc.want, got)
			}
			if out == tc.want {
				t.Fatalf("expected styling to be applied for %q", tc.in)
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
		if got := ansi.Strip(out); got != "text" {
			t.Fatalf("expected stripped output to omit bold delimiters\ngot: %q", got)
		}
		if !strings.Contains(out, "\x1b[1m") && !strings.Contains(out, "\x1b[1;") {
			t.Fatalf("expected bold SGR in output, got: %q", out)
		}
		if strings.Contains(out, "\x1b[3m") {
			t.Fatalf("expected no italic SGR in bold output, got: %q", out)
		}
	})
}

func TestFormatWithStyles_plainTextUsesBaseStyle(t *testing.T) {
	withANSIProfile(t, func() {
		in := "plain **bold** plain"
		base := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
		span := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
		out := FormatWithStyles(
			in,
			base,
			span,
		)
		if got := ansi.Strip(out); got != "plain bold plain" {
			t.Fatalf("expected stripped output to omit bold delimiters\ngot: %q", got)
		}

		// Verify that non-span segments are rendered with the base style, and
		// span segments are rendered with the span style. Avoid asserting on a
		// specific SGR sequence (ANSI16 vs ANSI256 vs TrueColor).
		if !strings.Contains(out, base.Render("plain ")) {
			t.Fatalf("expected base style to apply to leading plain segment, got: %q", out)
		}
		if !strings.Contains(out, span.Bold(true).Render("bold")) {
			t.Fatalf("expected span style to apply to bold span, got: %q", out)
		}
		if !strings.Contains(out, base.Render(" plain")) {
			t.Fatalf("expected base style to apply to trailing plain segment, got: %q", out)
		}
	})
}
