package history

import (
	"strings"
	"testing"
)

func TestRenderLogBody_prefixesEveryLine(t *testing.T) {
	out := renderLogBody("a\nb\nc", 80)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %#v", len(lines), lines)
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, logPrefix) {
			t.Fatalf("expected line %d to start with %q, got %q", i, logPrefix, line)
		}
	}
}

func TestRenderLogBody_ignoresTrailingNewline(t *testing.T) {
	out := renderLogBody("a\n", 80)
	if strings.Contains(out, "\n"+logPrefix) {
		t.Fatalf("expected trailing newline not to produce an extra %q line; got %q", logPrefix, out)
	}
}
