package history

import "testing"

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
