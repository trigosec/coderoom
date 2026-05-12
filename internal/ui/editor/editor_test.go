package editor

import "testing"

func TestResolve_prefersEDITOROverVISUAL(t *testing.T) {
	t.Setenv("EDITOR", "vim")
	t.Setenv("VISUAL", "nano")
	got, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() err: %v", err)
	}
	if got != "vim" {
		t.Fatalf("expected EDITOR to win, got %q", got)
	}
}

func TestResolve_usesVISUALWhenEDITORUnset(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "nano")
	got, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() err: %v", err)
	}
	if got != "nano" {
		t.Fatalf("expected VISUAL, got %q", got)
	}
}

func TestResolve_errorsWhenUnset(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	_, err := Resolve()
	if err == nil {
		t.Fatalf("expected error when editor unset")
	}
}

func TestOpenTempFileInEditor_errorsWhenUnset(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	_, err := OpenTempFileInEditor(Request{Purpose: PurposeCompose, TempPattern: "coderoom-compose-*.md"})
	if err == nil {
		t.Fatalf("expected error when editor unset")
	}
}

func TestOpenTempFileInEditor_requiresTempPatternExtension(t *testing.T) {
	t.Setenv("EDITOR", "true")
	t.Setenv("VISUAL", "")
	_, err := OpenTempFileInEditor(Request{
		Purpose:     PurposeCompose,
		TempPattern: "coderoom-compose-*",
	})
	if err == nil {
		t.Fatalf("expected error for missing temp pattern extension")
	}
}

func TestNormalizeContent_trimFinalNewline(t *testing.T) {
	if got := normalizeContent("a\n", true); got != "a" {
		t.Fatalf("expected final newline trimmed, got %q", got)
	}
	if got := normalizeContent("a\n\n", true); got != "a\n" {
		t.Fatalf("expected only one final newline trimmed, got %q", got)
	}
	if got := normalizeContent("a\n", false); got != "a\n" {
		t.Fatalf("expected content unchanged when trim disabled, got %q", got)
	}
}
