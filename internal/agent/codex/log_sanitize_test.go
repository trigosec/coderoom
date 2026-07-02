package codex

import "testing"

func TestSanitizeLogText_stripsANSI(t *testing.T) {
	in := "\x1b[2m2026-07-02T08:30:32Z\x1b[0m \x1b[31mERROR\x1b[0m test"
	got := sanitizeLogText(in)
	want := "2026-07-02T08:30:32Z ERROR test"
	if got != want {
		t.Fatalf("sanitizeLogText() = %q, want %q", got, want)
	}
}
