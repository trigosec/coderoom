package codex_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
	"github.com/trigosec/coderoom/internal/transcript"
)

func TestReplayFixtures(t *testing.T) {
	paths := findTranscriptFixtures(t, "testdata/transcripts")
	if len(paths) == 0 {
		t.Fatal("no transcript fixtures found")
	}
	for _, path := range paths {
		path := path
		t.Run(transcriptCaseName(path), func(t *testing.T) {
			runTranscriptCase(t, path)
		})
	}
}

func runTranscriptCase(t *testing.T, path string) {
	t.Helper()

	file, err := readTranscriptFixture(path)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}

	repoRoot := mustRepoRoot(t)
	listener := newReplayApprovalListener(file.Expect.Approvals)
	command, args := replayCommand(t, path)
	client := codex.New(
		repoRoot,
		codex.WithAppServerCommand(command, args...),
		codex.WithApprovalListener(listener),
		codex.WithModel(file.Model),
	)
	startReplayClient(t, client)

	collector := &replayCollector{}
	anchor, err := client.Send(file.Input)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	collector.turnAnchor = anchor

	for {
		msg, err := client.Read()
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		collector.observe(msg)
		if msg.StreamID == anchor && msg.Mode == agent.ModeFlush {
			break
		}
	}

	if err := assertReplayExpectations(file.Expect, collector, listener); err != nil {
		t.Fatalf("assert replay expectations: %v", err)
	}
}

func findTranscriptFixtures(t *testing.T, root string) []string {
	t.Helper()

	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "output.transcript" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk transcripts: %v", err)
	}
	slices.Sort(paths)
	return paths
}

func transcriptCaseName(path string) string {
	dir := filepath.Dir(path)
	return filepath.ToSlash(strings.TrimPrefix(dir, "testdata/transcripts/"))
}

func readTranscriptFixture(path string) (transcript.File, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return transcript.File{}, fmt.Errorf("open %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	fixture, _, err := transcript.Read(file)
	if err != nil {
		return transcript.File{}, fmt.Errorf("parse %q: %w", path, err)
	}
	return fixture, nil
}

func replayCommand(t *testing.T, path string) (string, []string) {
	t.Helper()

	repoRoot := mustRepoRoot(t)
	transcriptPath, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("Abs(%q): %v", path, err)
	}
	return "go", []string{
		"run",
		filepath.Join(repoRoot, "cmd", "codex-replay"),
		"--transcript",
		transcriptPath,
	}
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", "..", ".."))
}

type replayCollector struct {
	turnAnchor agent.StreamID

	outputCount     int
	outputText      strings.Builder
	reasoningCount  int
	reasoningText   strings.Builder
	fileChangeCount int
	commandCount    int

	filePaths []string
	commands  []string
}

func (c *replayCollector) observe(msg agent.Message) {
	switch content := msg.Content.(type) {
	case agent.Output:
		if msg.StreamID == c.turnAnchor && msg.Mode == agent.ModeFlush {
			return
		}
		c.outputCount++
		c.outputText.WriteString(content.Text)
	case agent.Reasoning:
		c.reasoningCount++
		c.reasoningText.WriteString(content.Text)
	case agent.FileChangeSet:
		c.fileChangeCount++
		for _, change := range content.Changes {
			replayAppendUnique(&c.filePaths, change.Path)
		}
	case agent.Command:
		c.commandCount++
		if strings.TrimSpace(content.Command) != "" {
			replayAppendUnique(&c.commands, content.Command)
		}
	}
}

func replayAppendUnique(dst *[]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" || slices.Contains(*dst, value) {
		return
	}
	*dst = append(*dst, value)
}

type replayApprovalListener struct {
	expected []transcript.ApprovalExpectation
	observed []transcript.ApprovalExpectation
	err      error
}

func newReplayApprovalListener(expected []transcript.ApprovalExpectation) *replayApprovalListener {
	return &replayApprovalListener{expected: expected}
}

func (l *replayApprovalListener) Decide(_ context.Context, req agent.ApprovalRequest) (agent.ApprovalDecision, error) {
	choice := agent.OptionDecline
	index := len(l.observed)
	if index >= len(l.expected) {
		if l.err == nil {
			l.err = fmt.Errorf("unexpected approval kind %q", req.Kind)
		}
	} else {
		expected := l.expected[index]
		choice = expected.Decision
		if expected.Kind != req.Kind && l.err == nil {
			l.err = fmt.Errorf("approval %d kind = %q, want %q", index, req.Kind, expected.Kind)
		}
		if !slices.Contains(req.Options, choice) && l.err == nil {
			l.err = fmt.Errorf("approval %d decision %q not allowed by options %v", index, choice, req.Options)
		}
	}
	l.observed = append(l.observed, transcript.ApprovalExpectation{
		Kind:     req.Kind,
		Decision: choice,
	})
	return agent.ApprovalDecision{Choice: choice}, nil
}

func assertReplayExpectations(expect transcript.Expect, collector *replayCollector, listener *replayApprovalListener) error {
	if listener.err != nil {
		return listener.err
	}
	if err := assertReplayApprovals(expect.Approvals, listener.observed); err != nil {
		return err
	}
	if err := assertReplayTextExpectation("output", expect.Output, collector.outputCount, collector.outputText.String()); err != nil {
		return err
	}
	if err := assertReplayTextExpectation("reasoning", expect.Reasoning, collector.reasoningCount, collector.reasoningText.String()); err != nil {
		return err
	}
	if err := assertReplayFileChangeExpectation(expect.FileChange, collector); err != nil {
		return err
	}
	if err := assertReplayCommandExpectation(expect.Command, collector); err != nil {
		return err
	}
	return nil
}

func assertReplayApprovals(expected, observed []transcript.ApprovalExpectation) error {
	if len(observed) != len(expected) {
		return fmt.Errorf("approval count = %d, want %d", len(observed), len(expected))
	}
	if !reflect.DeepEqual(observed, expected) {
		return fmt.Errorf("approvals = %v, want %v", observed, expected)
	}
	return nil
}

func assertReplayTextExpectation(label string, expected transcript.TextExpectation, count int, content string) error {
	if count != expected.NumMessages {
		return fmt.Errorf("%s.num_messages = %d, want %d", label, count, expected.NumMessages)
	}
	if content != expected.Content {
		return fmt.Errorf("%s.content = %q, want %q", label, content, expected.Content)
	}
	return nil
}

func assertReplayFileChangeExpectation(expected transcript.FileChangeExpectation, collector *replayCollector) error {
	if collector.fileChangeCount != expected.NumMessages {
		return fmt.Errorf("file_change.num_messages = %d, want %d", collector.fileChangeCount, expected.NumMessages)
	}
	if !reflect.DeepEqual(collector.filePaths, expected.Files) {
		return fmt.Errorf("file_change.files = %v, want %v", collector.filePaths, expected.Files)
	}
	return nil
}

func assertReplayCommandExpectation(expected transcript.CommandExpectation, collector *replayCollector) error {
	if collector.commandCount != expected.NumMessages {
		return fmt.Errorf("command.num_messages = %d, want %d", collector.commandCount, expected.NumMessages)
	}
	if !reflect.DeepEqual(collector.commands, expected.Executed) {
		return fmt.Errorf("command.executed = %v, want %v", collector.commands, expected.Executed)
	}
	return nil
}

func startReplayClient(t *testing.T, c agent.Agent) {
	t.Helper()
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
}
