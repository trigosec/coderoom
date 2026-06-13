package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trigosec/coderoom/internal/agent/codex"
)

func TestReadPromptScenario(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "agent", "codex", "testdata", "transcripts", "0.133.0", "approvals-file-change", "prompt.md")
	dir := filepath.Dir(path)

	got, err := readScenario(dir)
	if err != nil {
		t.Fatalf("readScenario: %v", err)
	}
	if got.Config.Model != "gpt-5.4" {
		t.Fatalf("model = %q, want gpt-5.4", got.Config.Model)
	}
	if got.Config.AskForApproval != codex.AskOnRequest {
		t.Fatalf("ask_for_approval = %q, want %q", got.Config.AskForApproval, codex.AskOnRequest)
	}
	if got.Config.Sandbox != codex.SandboxReadOnly {
		t.Fatalf("sandbox = %q, want %q", got.Config.Sandbox, codex.SandboxReadOnly)
	}
	if got.Config.ReasoningEffort != codex.ReasoningDefault {
		t.Fatalf("reasoning_effort = %q, want empty default", got.Config.ReasoningEffort)
	}
	if got.Config.ReasoningSummary != codex.ReasoningSummaryDefault {
		t.Fatalf("reasoning_summary = %q, want empty default", got.Config.ReasoningSummary)
	}
	if len(got.Actions) != 1 || got.Actions[0].Kind != "prompt" {
		t.Fatalf("actions = %#v, want single prompt", got.Actions)
	}
	if !strings.Contains(got.Actions[0].Text, "codex_file_approval_test.txt") {
		t.Fatalf("prompt = %q, want file name", got.Actions[0].Text)
	}
}

func TestReadPromptScenario_ReasoningSettings(t *testing.T) {
	path := writeScenarioFile(t, `---
model: gpt-5.4
reasoning_effort: xhigh
reasoning_summary: detailed
---
prompt`)

	got, err := readPromptScenario(path)
	if err != nil {
		t.Fatalf("readPromptScenario: %v", err)
	}
	if got.Config.ReasoningEffort != codex.ReasoningXHigh {
		t.Fatalf("reasoning_effort = %q, want %q", got.Config.ReasoningEffort, codex.ReasoningXHigh)
	}
	if got.Config.ReasoningSummary != codex.ReasoningSummaryDetailed {
		t.Fatalf("reasoning_summary = %q, want %q", got.Config.ReasoningSummary, codex.ReasoningSummaryDetailed)
	}
}

func TestReadConversationScenario(t *testing.T) {
	root := t.TempDir()
	writeNamedFile(t, filepath.Join(root, conversationFileName), `---
model: gpt-5.4
---
`)
	writeNamedFile(t, filepath.Join(root, "conversation-01.md"), `---
kind: notice
---
The magic word is PICASSO.`)
	writeNamedFile(t, filepath.Join(root, "conversation-02.md"), `Reply with a single word: done`)

	got, err := readScenario(root)
	if err != nil {
		t.Fatalf("readScenario: %v", err)
	}
	if got.Config.Model != "gpt-5.4" {
		t.Fatalf("model = %q, want gpt-5.4", got.Config.Model)
	}
	if len(got.Actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(got.Actions))
	}
	if got.Actions[0].Kind != "notice" || got.Actions[0].Text != "The magic word is PICASSO." {
		t.Fatalf("first action = %#v", got.Actions[0])
	}
	if got.Actions[1].Kind != "prompt" || got.Actions[1].Text != "Reply with a single word: done" {
		t.Fatalf("second action = %#v", got.Actions[1])
	}
}

func TestReadConversationScenario_RejectsUnknownActionKey(t *testing.T) {
	root := t.TempDir()
	writeNamedFile(t, filepath.Join(root, conversationFileName), "---\nmodel: gpt-5.4\n---\n")
	writeNamedFile(t, filepath.Join(root, "conversation-01.md"), `---
unexpected: true
---
hello`)

	if _, err := readScenario(root); err == nil {
		t.Fatal("readScenario succeeded, want error")
	}
}

func TestReadConversationScenario_RejectsMissingSequence(t *testing.T) {
	root := t.TempDir()
	writeNamedFile(t, filepath.Join(root, conversationFileName), "---\nmodel: gpt-5.4\n---\n")
	writeNamedFile(t, filepath.Join(root, "conversation-02.md"), "hello")

	if _, err := readScenario(root); err == nil {
		t.Fatal("readScenario succeeded, want error")
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

func writeScenarioFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), promptFileName)
	writeNamedFile(t, path, content)
	return path
}

func writeNamedFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
