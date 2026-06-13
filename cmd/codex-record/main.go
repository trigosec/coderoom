// Command codex-record records live Codex transcript fixtures from prompt.md
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
const promptFileName = "prompt.md"
const conversationFileName = "conversation.md"

type scenarioConfig struct {
	Model            string
	AskForApproval   codex.AskForApprovalPolicy
	Sandbox          codex.SandboxMode
	ReasoningEffort  codex.ReasoningEffort
	ReasoningSummary codex.ReasoningSummary
}

type scenario struct {
	Config  scenarioConfig
	Actions []transcript.Action
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

	filePaths         []string
	commands          []string
	approvals         []transcript.ApprovalExpectation
	noticeTurnFlushes int
}

const noticeTurnStreamID = agent.StreamID("codex:notice-turn")

func main() {
	cases, err := resolveSelection(transcriptRoot, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve fixture: %v\n", err)
		os.Exit(1)
	}

	for _, tc := range cases {
		input, readErr := readScenario(tc.dir)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "read scenario: %v\n", readErr)
			os.Exit(1)
		}

		if err := recordCase(tc.dir, tc.version, tc.name, input); err != nil {
			fmt.Fprintf(os.Stderr, "record transcript: %v\n", err)
			os.Exit(1)
		}
	}
}

func recordCase(caseDir, version, testCase string, input scenario) error {
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
	client, err := startClient(workDir, testCase, input.Config, recorder, collector)
	if err != nil {
		return err
	}
	defer stopClient(client)

	if err := runScenario(client, input.Actions, collector); err != nil {
		return err
	}
	fixture := buildFixture(version, testCase, input, collector)
	return writeTranscript(filepath.Join(caseDir, "output.transcript"), fixture, recorder.Steps())
}

func startClient(workDir, _ string, input scenarioConfig, recorder *transcript.Observer, collector *collector) (*codex.Client, error) {
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

func runScenario(client *codex.Client, actions []transcript.Action, collector *collector) error {
	for _, action := range actions {
		if err := runAction(client, action, collector); err != nil {
			return err
		}
	}
	return nil
}

func runAction(client *codex.Client, action transcript.Action, collector *collector) error {
	anchor, err := sendAction(client, action)
	if err != nil {
		return err
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

func sendAction(client *codex.Client, action transcript.Action) (agent.StreamID, error) {
	switch action.Kind {
	case "", "prompt":
		anchor, err := client.Send(action.Text)
		if err != nil {
			return "", fmt.Errorf("send prompt: %w", err)
		}
		return anchor, nil
	case "notice":
		anchor, err := client.SendNotice(action.Text)
		if err != nil {
			return "", fmt.Errorf("send notice: %w", err)
		}
		return anchor, nil
	default:
		return "", fmt.Errorf("unknown action kind %q", action.Kind)
	}
}

func buildFixture(version, testCase string, input scenario, collector *collector) transcript.File {
	var noticeExpect *transcript.NoticeExpectation
	if scenarioHasNotice(input.Actions) {
		noticeExpect = &transcript.NoticeExpectation{
			NumTurnFlushes: collector.noticeTurnFlushes,
		}
	}
	return transcript.File{
		Name:         testCase,
		CodexVersion: version,
		Model:        input.Config.Model,
		Actions:      input.Actions,
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
			Notice:    noticeExpect,
			Approvals: collector.approvals,
		},
	}
}

func scenarioHasNotice(actions []transcript.Action) bool {
	for _, action := range actions {
		if action.Kind == "notice" {
			return true
		}
	}
	return false
}

func (c *collector) observe(msg agent.Message) {
	if c.observeLifecycleFlush(msg) {
		return
	}
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

func (c *collector) observeLifecycleFlush(msg agent.Message) bool {
	if msg.StreamID == noticeTurnStreamID && msg.Mode == agent.ModeFlush {
		c.noticeTurnFlushes++
		return true
	}
	if msg.StreamID == c.turnAnchor && msg.Mode == agent.ModeFlush {
		return true
	}
	return false
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

func readScenario(caseDir string) (scenario, error) {
	hasPrompt := fileExists(filepath.Join(caseDir, promptFileName))
	hasConversation := fileExists(filepath.Join(caseDir, conversationFileName))
	switch {
	case hasPrompt && hasConversation:
		return scenario{}, errors.New("found both prompt.md and conversation.md")
	case hasPrompt:
		return readPromptScenario(filepath.Join(caseDir, promptFileName))
	case hasConversation:
		return readConversationScenario(caseDir)
	default:
		return scenario{}, errors.New("missing prompt.md or conversation.md")
	}
}

func readPromptScenario(path string) (scenario, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return scenario{}, fmt.Errorf("open prompt file %q: %w", path, err)
	}
	defer closeFile(f)
	config, body, err := parseMarkdownScenarioFile(f)
	if err != nil {
		return scenario{}, err
	}
	if strings.TrimSpace(body) == "" {
		return scenario{}, errors.New("empty prompt body")
	}
	return scenario{
		Config: config,
		Actions: []transcript.Action{{
			Kind: "prompt",
			Text: strings.TrimSpace(body),
		}},
	}, nil
}

func readConversationScenario(caseDir string) (scenario, error) {
	config, err := readConversationConfig(filepath.Join(caseDir, conversationFileName))
	if err != nil {
		return scenario{}, err
	}
	actionFiles, err := resolveConversationActionFiles(caseDir)
	if err != nil {
		return scenario{}, err
	}
	actions := make([]transcript.Action, 0, len(actionFiles))
	for _, path := range actionFiles {
		action, err := readConversationAction(path)
		if err != nil {
			return scenario{}, err
		}
		actions = append(actions, action)
	}
	return scenario{Config: config, Actions: actions}, nil
}

func readConversationConfig(path string) (scenarioConfig, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return scenarioConfig{}, fmt.Errorf("open conversation file %q: %w", path, err)
	}
	defer closeFile(f)
	config, body, err := parseMarkdownScenarioFile(f)
	if err != nil {
		return scenarioConfig{}, err
	}
	if strings.TrimSpace(body) != "" {
		return scenarioConfig{}, errors.New("conversation.md body must be empty")
	}
	return config, nil
}

func readConversationAction(path string) (transcript.Action, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return transcript.Action{}, fmt.Errorf("open conversation action %q: %w", path, err)
	}
	defer closeFile(f)
	action, err := parseConversationActionFile(f)
	if err != nil {
		return transcript.Action{}, fmt.Errorf("parse conversation action %q: %w", path, err)
	}
	return action, nil
}

func parseMarkdownScenarioFile(r io.Reader) (scenarioConfig, string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return scenarioConfig{}, "", fmt.Errorf("read scenario file: %w", err)
	}
	parts := bytes.SplitN(data, []byte("\n---\n"), 2)
	if len(parts) != 2 {
		return scenarioConfig{}, "", errors.New("missing front matter delimiter")
	}
	input, err := parseInputFrontMatter(parts[0])
	if err != nil {
		return scenarioConfig{}, "", err
	}
	return input, string(parts[1]), nil
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

func parseInputFrontMatter(raw []byte) (scenarioConfig, error) {
	var input scenarioConfig
	for _, line := range strings.Split(string(raw), "\n") {
		if err := applyInputFrontMatterLine(&input, line); err != nil {
			return scenarioConfig{}, err
		}
	}
	return input, nil
}

func applyInputFrontMatterLine(input *scenarioConfig, line string) error {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || trimmed == "---" || strings.HasPrefix(trimmed, "#") {
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

func assignInputFrontMatterValue(input *scenarioConfig, key, decoded string) error {
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

func parseConversationActionFile(r io.Reader) (transcript.Action, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return transcript.Action{}, fmt.Errorf("read conversation action file: %w", err)
	}
	parts := bytes.SplitN(data, []byte("\n---\n"), 2)
	if len(parts) == 1 {
		body := strings.TrimSpace(string(parts[0]))
		if body == "" {
			return transcript.Action{}, errors.New("empty conversation action body")
		}
		return transcript.Action{Kind: "prompt", Text: body}, nil
	}
	action, err := parseConversationActionFrontMatter(parts[0])
	if err != nil {
		return transcript.Action{}, err
	}
	action.Text = strings.TrimSpace(string(parts[1]))
	if action.Text == "" {
		return transcript.Action{}, errors.New("empty conversation action body")
	}
	if action.Kind == "" {
		action.Kind = "prompt"
	}
	return action, nil
}

func parseConversationActionFrontMatter(raw []byte) (transcript.Action, error) {
	var action transcript.Action
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "---" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, decoded, err := parseInputFrontMatterScalar(trimmed)
		if err != nil {
			return transcript.Action{}, err
		}
		switch key {
		case "kind":
			action.Kind = decoded
		default:
			return transcript.Action{}, fmt.Errorf("unknown conversation action key %q", key)
		}
	}
	return action, nil
}

func resolveConversationActionFiles(caseDir string) ([]string, error) {
	names, err := listDirNamesOrFiles(caseDir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, name := range names {
		if strings.HasPrefix(name, "conversation-") && strings.HasSuffix(name, ".md") {
			files = append(files, name)
		}
	}
	slices.Sort(files)
	if len(files) == 0 {
		return nil, errors.New("missing conversation action files")
	}
	for index, name := range files {
		expected := fmt.Sprintf("conversation-%02d.md", index+1)
		if name != expected {
			return nil, fmt.Errorf("expected %s, found %s", expected, name)
		}
	}
	paths := make([]string, 0, len(files))
	for _, name := range files {
		paths = append(paths, filepath.Join(caseDir, name))
	}
	return paths, nil
}

func listDirNamesOrFiles(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read directory %q: %w", root, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	return names, nil
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
