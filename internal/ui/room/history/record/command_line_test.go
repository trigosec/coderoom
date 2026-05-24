package record

import (
	"strings"
	"testing"
)

func TestRenderCommandLine_longCmdWrapsWithoutRepeatingDollarPrefix(t *testing.T) {
	// "  $ " is 4 columns wide; with width=10, contentWidth=6.
	out := renderCommandLine("  $ ", "abcdefghij", 10)
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapped output to span multiple lines, got %q", out)
	}
	if lines[0] != "  $ abcdef" {
		t.Errorf("unexpected first line, got %q", lines[0])
	}
	if lines[1] != "    ghij" {
		t.Errorf("unexpected continuation indent/content, got %q", lines[1])
	}
	for i, line := range lines[1:] {
		if strings.HasPrefix(line, "  $ ") {
			t.Errorf("continuation line %d should not repeat command prefix, got %q", i+1, line)
		}
	}
}
