package transcript

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/trigosec/coderoom/internal/agent"
)

func TestRead_FromTestdata(t *testing.T) {
	path := filepath.Join("testdata", "approvals_file_change.transcript")
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}

	gotFile, gotSteps, err := Read(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	assertTranscriptFile(t, gotFile)
	assertTranscriptSteps(t, gotSteps)
}

func TestWrite_MatchesTestdata(t *testing.T) {
	file := File{
		Name:         "approvals_file_change",
		CodexVersion: "0.133.0",
		Model:        "gpt-5.4",
		Input:        "prompt",
		Expect: Expect{
			Output:     TextExpectation{NumMessages: 0, Content: ""},
			Reasoning:  TextExpectation{NumMessages: 1, Content: "think"},
			FileChange: FileChangeExpectation{NumMessages: 2, Files: []string{"a.txt"}},
			Command:    CommandExpectation{NumMessages: 1, Executed: []string{"ls -la"}},
			Approvals:  []ApprovalExpectation{{Kind: agent.ApprovalFileChange, Decision: agent.OptionAccept}},
		},
	}
	steps := []Step{
		{Kind: "recv", Match: map[string]any{"method": "initialize"}},
		{Kind: "send", Message: map[string]any{"id": 1.0, "result": map[string]any{"ok": true}}},
	}

	var buf bytes.Buffer
	if err := Write(&buf, file, steps); err != nil {
		t.Fatalf("Write: %v", err)
	}

	want, err := os.ReadFile(filepath.Clean(filepath.Join("testdata", "approvals_file_change.transcript")))
	if err != nil {
		t.Fatalf("ReadFile(golden): %v", err)
	}
	if diff := bytes.Compare(buf.Bytes(), want); diff != 0 {
		t.Fatalf("written transcript did not match golden fixture\n--- got ---\n%s\n--- want ---\n%s", buf.String(), string(want))
	}
}

func TestRead_MissingDelimiter(t *testing.T) {
	data, err := os.ReadFile(filepath.Clean(filepath.Join("testdata", "missing_delimiter.transcript")))
	if err != nil {
		t.Fatalf("ReadFile(malformed): %v", err)
	}
	if _, _, err := Read(bytes.NewReader(data)); err == nil {
		t.Fatal("Read succeeded, want error for missing delimiter")
	}
}

func assertTranscriptFile(t *testing.T, gotFile File) {
	t.Helper()
	if gotFile.Name != "approvals_file_change" {
		t.Fatalf("name = %q, want approvals_file_change", gotFile.Name)
	}
	if gotFile.CodexVersion != "0.133.0" {
		t.Fatalf("codex version = %q, want 0.133.0", gotFile.CodexVersion)
	}
	if gotFile.Model != "gpt-5.4" {
		t.Fatalf("model = %q, want gpt-5.4", gotFile.Model)
	}
	if gotFile.Expect.FileChange.NumMessages != 2 {
		t.Fatalf("file change count = %d, want 2", gotFile.Expect.FileChange.NumMessages)
	}
	if len(gotFile.Expect.Command.Executed) != 1 || gotFile.Expect.Command.Executed[0] != "ls -la" {
		t.Fatalf("executed commands = %#v, want [\"ls -la\"]", gotFile.Expect.Command.Executed)
	}
	if len(gotFile.Expect.Approvals) != 1 || gotFile.Expect.Approvals[0].Decision != agent.OptionAccept {
		t.Fatalf("approvals = %#v", gotFile.Expect.Approvals)
	}
}

func assertTranscriptSteps(t *testing.T, gotSteps []Step) {
	t.Helper()
	if len(gotSteps) != 2 {
		t.Fatalf("steps = %d, want 2", len(gotSteps))
	}
}
