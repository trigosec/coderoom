package history

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/trigosec/coderoom/internal/agent"
)

func TestRenderReasoning_bodyUsesSystemStyleExceptEmphasis(t *testing.T) {
	withANSIProfile(t, func() {
		span := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
		out := renderReasoning(Record{
			Kind:  RecordKindReasoning,
			Alias: "alice",
			Msg: &agent.Message{
				StreamID: "s1",
				Mode:     agent.ModeStream,
				Content:  agent.Reasoning{Text: "plain **bold** plain"},
			},
		}, 200, func(alias string) string {
			if alias != "alice" {
				return ""
			}
			return "6"
		})

		if got, want := ansi.Strip(out), "◈ alice (thinking)\n\n  plain **bold** plain"; got != want {
			t.Fatalf("unexpected stripped output:\nwant: %q\n got: %q", want, got)
		}

		// Ensure the participant color is used for emphasis spans, but not for the
		// surrounding plain text.
		if !strings.Contains(out, span.Bold(true).Render("**bold**")) {
			t.Fatalf("expected participant color to apply to bold span; got %q", out)
		}
		if strings.Contains(out, span.Render("plain")) {
			t.Fatalf("expected participant color to not apply to plain text; got %q", out)
		}
	})
}
