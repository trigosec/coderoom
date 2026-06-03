package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trigosec/coderoom/internal/agent/codex"
)

func TestParseInputFile(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "agent", "codex", "testdata", "transcripts", "0.133.0", "approvals-file-change", "input.md")
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}

	input, err := parseInputFile(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("parseInputFile: %v", err)
	}
	if input.Model != "gpt-5.4" {
		t.Fatalf("model = %q, want gpt-5.4", input.Model)
	}
	if input.AskForApproval != codex.AskOnRequest {
		t.Fatalf("ask_for_approval = %q, want %q", input.AskForApproval, codex.AskOnRequest)
	}
	if input.Sandbox != codex.SandboxReadOnly {
		t.Fatalf("sandbox = %q, want %q", input.Sandbox, codex.SandboxReadOnly)
	}
	if !strings.Contains(input.Prompt, "codex_file_approval_test.txt") {
		t.Fatalf("prompt = %q, want file name", input.Prompt)
	}
}

func TestResolveSelection(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "agent", "codex", "testdata", "transcripts")

	cases, err := resolveSelection(root, []string{"0.133.0", "approvals-file-change"})
	if err != nil {
		t.Fatalf("resolveSelection: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(cases))
	}
	if cases[0].version != "0.133.0" || cases[0].name != "approvals-file-change" {
		t.Fatalf("case = %#v, want version=%q name=%q", cases[0], "0.133.0", "approvals-file-change")
	}
}

func TestResolveSelection_AllCases(t *testing.T) {
	root := t.TempDir()
	mustMakeCase(t, root, "0.133.0", "alpha")
	mustMakeCase(t, root, "0.133.0", "beta")
	mustMakeCase(t, root, "0.200.0", "gamma")

	cases, err := resolveSelection(root, nil)
	if err != nil {
		t.Fatalf("resolveSelection: %v", err)
	}
	if len(cases) != 3 {
		t.Fatalf("cases = %d, want 3", len(cases))
	}
	if cases[0].version != "0.133.0" || cases[0].name != "alpha" {
		t.Fatalf("first case = %#v", cases[0])
	}
	if cases[2].version != "0.200.0" || cases[2].name != "gamma" {
		t.Fatalf("last case = %#v", cases[2])
	}
}

func TestResolveSelection_VersionCases(t *testing.T) {
	root := t.TempDir()
	mustMakeCase(t, root, "0.133.0", "alpha")
	mustMakeCase(t, root, "0.133.0", "beta")
	mustMakeCase(t, root, "0.200.0", "gamma")

	cases, err := resolveSelection(root, []string{"0.133.0"})
	if err != nil {
		t.Fatalf("resolveSelection: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("cases = %d, want 2", len(cases))
	}
	if cases[0].version != "0.133.0" || cases[0].name != "alpha" {
		t.Fatalf("first case = %#v", cases[0])
	}
	if cases[1].version != "0.133.0" || cases[1].name != "beta" {
		t.Fatalf("second case = %#v", cases[1])
	}
}

func mustMakeCase(t *testing.T, root, version, name string) {
	t.Helper()
	caseDir := filepath.Join(root, version, name)
	if err := os.MkdirAll(caseDir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
}
