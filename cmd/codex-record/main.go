// Command codex-record records live Codex transcript fixtures from input.md
// cases under internal/agent/codex/testdata/transcripts.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
	"github.com/trigosec/coderoom/internal/transcript"
)

const transcriptRoot = "internal/agent/codex/testdata/transcripts"

type inputFile struct {
	Model            string
	AskForApproval   codex.AskForApprovalPolicy
	Sandbox          codex.SandboxMode
	ReasoningEffort  codex.ReasoningEffort
	ReasoningSummary codex.ReasoningSummary
	Prompt           string
}

type transcriptCase struct {
	version string
	name    string
	dir     string
}

type collector struct {
	turnAnchor agent.StreamID

	outputCount      int
	outputText       strings.Builder
	reasoningCount   int
	reasoningText    strings.Builder
	reasoningStreams map[agent.StreamID]toolStreamState
	fileChangeCount  int
	commandCount     int

	filePaths []string
	commands  []string
	approvals []transcript.ApprovalExpectation
}

func main() {
	cases, err := resolveSelection(transcriptRoot, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve fixture: %v\n", err)
		os.Exit(1)
	}

	for _, tc := range cases {
		input, readErr := readInputFile(filepath.Join(tc.dir, "input.md"))
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "read input.md: %v\n", readErr)
			os.Exit(1)
		}

		if err := recordCase(tc.dir, tc.version, tc.name, input); err != nil {
			fmt.Fprintf(os.Stderr, "record transcript: %v\n", err)
			os.Exit(1)
		}
	}
}

func recordCase(caseDir, version, testCase string, input inputFile) error {
	workDir, err := os.MkdirTemp("", "codex-record-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer cleanupDir(workDir)

	restoreEnv, err := setCodexVersion(version)
	if err != nil {
		return err
	}
	defer restoreEnv()

	recorder := transcript.NewObserver()
	collector := &collector{}
	client, err := startClient(workDir, testCase, input, recorder, collector)
	if err != nil {
		return err
	}
	defer stopClient(client)

	if err := runPrompt(client, input.Prompt, collector); err != nil {
		return err
	}
	fixture := buildFixture(version, testCase, input, collector)
	return writeTranscript(filepath.Join(caseDir, "output.transcript"), fixture, recorder.Steps())
}

func startClient(workDir, _ string, input inputFile, recorder *transcript.Observer, collector *collector) (*codex.Client, error) {
	listener := approvalListenerFunc(func(req agent.ApprovalRequest) agent.ApprovalDecision {
		choice := chooseApproval(req.Options)
		collector.approvals = append(collector.approvals, transcript.ApprovalExpectation{
			Kind:     req.Kind,
			Decision: choice,
		})
		return agent.ApprovalDecision{Choice: choice}
	})
	opts := []codex.Option{
		codex.WithObserver(recorder),
		codex.WithApprovalListener(listener),
		codex.WithModel(input.Model),
		codex.WithAskForApprovalPolicy(input.AskForApproval),
		codex.WithSandboxMode(input.Sandbox),
		codex.WithReasoningEffort(input.ReasoningEffort),
		codex.WithReasoningSummary(input.ReasoningSummary),
	}
	client := codex.New(workDir, opts...)
	if err := client.Start(); err != nil {
		return nil, fmt.Errorf("start client: %w", err)
	}
	return client, nil
}

func runPrompt(client *codex.Client, prompt string, collector *collector) error {
	anchor, err := client.Send(prompt)
	if err != nil {
		return fmt.Errorf("send prompt: %w", err)
	}
	collector.turnAnchor = anchor
	for {
		msg, err := client.Read()
		if err != nil {
			return fmt.Errorf("read message: %w", err)
		}
		collector.observe(msg)
		if msg.StreamID == anchor && msg.Mode == agent.ModeFlush {
			return nil
		}
	}
}

func buildFixture(version, testCase string, input inputFile, collector *collector) transcript.File {
	return transcript.File{
		Name:         testCase,
		CodexVersion: version,
		Model:        input.Model,
		Input:        input.Prompt,
		Expect: transcript.Expect{
			Output: transcript.TextExpectation{NumMessages: collector.outputCount, Content: collector.outputText.String()},
			Reasoning: transcript.ReasoningExpectation{
				NumMessages: collector.reasoningCount,
				Content:     collector.reasoningText.String(),
				NumStreams:  collector.reasoningNumStreams(),
				AllFlushed:  collector.reasoningAllFlushed(),
			},
			FileChange: transcript.FileChangeExpectation{
				NumMessages: collector.fileChangeCount,
				Files:       collector.filePaths,
			},
			Command: transcript.CommandExpectation{
				NumMessages: collector.commandCount,
				Executed:    collector.commands,
			},
			Approvals: collector.approvals,
		},
	}
}

func (c *collector) observe(msg agent.Message) {
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
		c.observeReasoningStream(msg)
	case agent.FileChangeSet:
		c.fileChangeCount++
		for _, change := range content.Changes {
			appendUnique(&c.filePaths, change.Path)
		}
	case agent.Command:
		c.commandCount++
		if strings.TrimSpace(content.Command) != "" {
			appendUnique(&c.commands, content.Command)
		}
	}
}

func (c *collector) observeReasoningStream(msg agent.Message) {
	if c.reasoningStreams == nil {
		c.reasoningStreams = make(map[agent.StreamID]toolStreamState)
	}
	state := c.reasoningStreams[msg.StreamID]
	if msg.Mode == agent.ModeStream {
		state.sawStream = true
	}
	if msg.Mode == agent.ModeFlush {
		state.sawFlush = true
	}
	c.reasoningStreams[msg.StreamID] = state
}

func (c *collector) reasoningNumStreams() int {
	count := 0
	for _, state := range c.reasoningStreams {
		if state.sawStream {
			count++
		}
	}
	return count
}

func (c *collector) reasoningAllFlushed() bool {
	for _, state := range c.reasoningStreams {
		if state.sawStream && !state.sawFlush {
			return false
		}
	}
	return true
}

type approvalListenerFunc func(req agent.ApprovalRequest) agent.ApprovalDecision

type toolStreamState struct {
	sawStream bool
	sawFlush  bool
}

func (f approvalListenerFunc) Decide(_ context.Context, req agent.ApprovalRequest) (agent.ApprovalDecision, error) {
	return f(req), nil
}

func chooseApproval(options []agent.ApprovalOption) agent.ApprovalOption {
	preferred := []agent.ApprovalOption{
		agent.OptionAccept,
		agent.OptionAcceptForSession,
	}
	for _, choice := range preferred {
		if slices.Contains(options, choice) {
			return choice
		}
	}
	if len(options) > 0 {
		return options[0]
	}
	return agent.OptionDecline
}

func writeTranscript(path string, file transcript.File, steps []transcript.Step) error {
	finalPath := filepath.Clean(path)
	var buf bytes.Buffer
	if err := transcript.Write(&buf, file, steps); err != nil {
		return fmt.Errorf("encode transcript: %w", err)
	}
	tmpPath := filepath.Clean(finalPath + ".tmp")
	if err := os.WriteFile(tmpPath, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write temp transcript: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("rename transcript: %w", err)
	}
	return nil
}

func setCodexVersion(version string) (func(), error) {
	const key = "CODEX_VERSION_OVERRIDE"
	old, hadOld := os.LookupEnv(key)
	if err := os.Setenv(key, version); err != nil {
		return nil, fmt.Errorf("set codex version override: %w", err)
	}
	return func() {
		if hadOld {
			if err := os.Setenv(key, old); err != nil {
				fmt.Fprintf(os.Stderr, "restore %s: %v\n", key, err)
			}
			return
		}
		if err := os.Unsetenv(key); err != nil {
			fmt.Fprintf(os.Stderr, "unset %s: %v\n", key, err)
		}
	}, nil
}

func readInputFile(path string) (inputFile, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return inputFile{}, fmt.Errorf("open input file %q: %w", path, err)
	}
	defer closeFile(f)
	return parseInputFile(f)
}

func parseInputFile(r io.Reader) (inputFile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return inputFile{}, fmt.Errorf("read input file: %w", err)
	}
	parts := bytes.SplitN(data, []byte("\n---\n"), 2)
	if len(parts) != 2 {
		return inputFile{}, errors.New("missing front matter delimiter")
	}
	input, err := parseInputFrontMatter(parts[0])
	if err != nil {
		return inputFile{}, err
	}
	input.Prompt = strings.TrimSpace(string(parts[1]))
	if input.Prompt == "" {
		return inputFile{}, errors.New("empty prompt body")
	}
	return input, nil
}

func appendUnique(dst *[]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" || slices.Contains(*dst, value) {
		return
	}
	*dst = append(*dst, value)
}

func unquoteScalar(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value[0] != '"' {
		return value, nil
	}
	decoded, err := strconv.Unquote(value)
	if err != nil {
		return "", fmt.Errorf("parse quoted value: %w", err)
	}
	return decoded, nil
}

func resolveSelection(root string, args []string) ([]transcriptCase, error) {
	if len(args) == 0 {
		return resolveAllCases(root)
	}
	if len(args) == 1 {
		return resolveVersionCases(root, args[0])
	}
	if len(args) == 2 {
		version, testCase := args[0], args[1]
		return []transcriptCase{{
			version: version,
			name:    testCase,
			dir:     filepath.Join(root, version, testCase),
		}}, nil
	}
	return nil, errors.New("usage: codex-record [<codex-version> [<test-case>]]")
}

func listDirNames(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read directory %q: %w", root, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

func parseInputFrontMatter(raw []byte) (inputFile, error) {
	var input inputFile
	for _, line := range strings.Split(string(raw), "\n") {
		if err := applyInputFrontMatterLine(&input, line); err != nil {
			return inputFile{}, err
		}
	}
	return input, nil
}

func applyInputFrontMatterLine(input *inputFile, line string) error {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || trimmed == "---" {
		return nil
	}
	key, decoded, err := parseInputFrontMatterScalar(trimmed)
	if err != nil {
		return err
	}
	return assignInputFrontMatterValue(input, key, decoded)
}

func parseInputFrontMatterScalar(trimmed string) (string, string, error) {
	key, value, ok := strings.Cut(trimmed, ":")
	if !ok {
		return "", "", fmt.Errorf("parse front matter line %q", trimmed)
	}
	normalizedKey := strings.TrimSpace(key)
	decoded, err := unquoteScalar(strings.TrimSpace(value))
	if err != nil {
		return "", "", fmt.Errorf("decode front matter value for %q: %w", normalizedKey, err)
	}
	return normalizedKey, decoded, nil
}

func assignInputFrontMatterValue(input *inputFile, key, decoded string) error {
	switch key {
	case "model":
		input.Model = decoded
	case "ask_for_approval":
		input.AskForApproval = codex.AskForApprovalPolicy(decoded)
	case "sandbox":
		input.Sandbox = codex.SandboxMode(decoded)
	case "reasoning_effort":
		input.ReasoningEffort = codex.ReasoningEffort(decoded)
	case "reasoning_summary":
		input.ReasoningSummary = codex.ReasoningSummary(decoded)
	default:
		return fmt.Errorf("unknown front matter key %q", key)
	}
	return nil
}

func resolveAllCases(root string) ([]transcriptCase, error) {
	versions, err := listDirNames(root)
	if err != nil {
		return nil, err
	}
	var cases []transcriptCase
	for _, version := range versions {
		versionCases, err := resolveVersionCases(root, version)
		if err != nil {
			return nil, err
		}
		cases = append(cases, versionCases...)
	}
	return cases, nil
}

func resolveVersionCases(root, version string) ([]transcriptCase, error) {
	names, err := listDirNames(filepath.Join(root, version))
	if err != nil {
		return nil, err
	}
	cases := make([]transcriptCase, 0, len(names))
	for _, name := range names {
		cases = append(cases, transcriptCase{
			version: version,
			name:    name,
			dir:     filepath.Join(root, version, name),
		})
	}
	return cases, nil
}

func cleanupDir(path string) {
	if err := os.RemoveAll(path); err != nil {
		fmt.Fprintf(os.Stderr, "remove temp dir %q: %v\n", path, err)
	}
}

func stopClient(client *codex.Client) {
	if err := client.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "stop client: %v\n", err)
	}
}

func closeFile(f *os.File) {
	if err := f.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close file: %v\n", err)
	}
}
