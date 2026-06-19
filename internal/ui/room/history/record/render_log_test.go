package record

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderLogBody_prefixesEveryLine(t *testing.T) {
	r := Record{Kind: KindLog, Text: "a\nb\nc"}
	out := ansi.Strip(Render(r, RenderContext{Key: RenderKey{Mode: RenderViewport, Width: 80}}))
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %#v", len(lines), lines)
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "▸ ") {
			t.Fatalf("expected line %d to start with %q, got %q", i, "▸ ", line)
		}
	}
}

func TestRenderLogBody_ignoresTrailingNewline(t *testing.T) {
	r := Record{Kind: KindLog, Text: "a\n"}
	out := ansi.Strip(Render(r, RenderContext{Key: RenderKey{Mode: RenderViewport, Width: 80}}))
	if strings.Contains(out, "\n▸ ") {
		t.Fatalf("expected trailing newline not to produce an extra %q line; got %q", "▸ ", out)
	}
}
