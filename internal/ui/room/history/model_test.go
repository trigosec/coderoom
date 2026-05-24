package history

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	rec "github.com/trigosec/coderoom/internal/ui/room/history/record"
)

func TestResolveColor_activeReturnsFromLookup(t *testing.T) {
	m := New(func(alias string) string {
		if alias == "ada" {
			return "#ff0000"
		}
		return ""
	}, "#6b7280")
	if got := m.resolveColor("ada"); got != "#ff0000" {
		t.Errorf("want #ff0000, got %q", got)
	}
}

func TestResolveColor_departedReturnsConfiguredColor(t *testing.T) {
	const grey = "#6b7280"
	m := New(nil, grey)
	m = m.MarkDeparted("ada")
	if got := m.resolveColor("ada"); got != grey {
		t.Errorf("want %q, got %q", grey, got)
	}
}

func TestResolveColor_departedTakesPrecedenceOverActive(t *testing.T) {
	const grey = "#6b7280"
	m := New(func(string) string { return "#ff0000" }, grey)
	m = m.MarkDeparted("ada")
	if got := m.resolveColor("ada"); got != grey {
		t.Errorf("want departed color %q, got %q", grey, got)
	}
}

func TestJoinRenderedForViewport_insertsBlankLineBetweenNonSystemRecords(t *testing.T) {
	out := joinRenderedForViewport([]rec.Record{
		{Kind: rec.KindUserInput},
		{Kind: rec.KindAgentOutput},
	}, []string{"a", "b"})
	if strings.Count(out, "\n") != 2 {
		t.Fatalf("expected 2 newlines between non-system records, got %d (%q)", strings.Count(out, "\n"), out)
	}
	if ansi.Strip(out) != "a\n\nb" {
		t.Fatalf("unexpected joined output: %q", ansi.Strip(out))
	}
}

func TestJoinRenderedForViewport_systemRecordsStayCompact(t *testing.T) {
	out := joinRenderedForViewport([]rec.Record{
		{Kind: rec.KindUserInput},
		{Kind: rec.KindSystem},
		{Kind: rec.KindSystem},
		{Kind: rec.KindAgentOutput},
	}, []string{"u", "s1", "s2", "a"})
	if ansi.Strip(out) != "u\ns1\ns2\n\na" {
		t.Fatalf("unexpected joined output: %q", ansi.Strip(out))
	}
}
