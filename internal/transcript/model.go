// Package transcript defines the transcript fixture model and IO helpers used
// by Codex recording and replay tooling.
package transcript

import "github.com/trigosec/coderoom/internal/agent"

// File is the on-disk transcript fixture: front matter plus replay steps.
type File struct {
	Name         string   `yaml:"name"`
	CodexVersion string   `yaml:"codex_version,omitempty"`
	Model        string   `yaml:"model,omitempty"`
	Input        string   `yaml:"input,omitempty"`
	Actions      []Action `yaml:"actions,omitempty"`
	Expect       Expect   `yaml:"expect"`
}

// Action is one normalized scenario action used to drive replay.
type Action struct {
	Kind string `yaml:"kind"`
	Text string `yaml:"text"`
}

// NormalizedActions returns the recorded action list, or a legacy single
// prompt action when older fixtures only carry Input.
func NormalizedActions(file File) []Action {
	if len(file.Actions) > 0 {
		return file.Actions
	}
	if file.Input == "" {
		return nil
	}
	return []Action{{Kind: "prompt", Text: file.Input}}
}

// Expect stores the high-level expectations captured during recording.
type Expect struct {
	Output     TextExpectation       `yaml:"output"`
	Reasoning  ReasoningExpectation  `yaml:"reasoning"`
	FileChange FileChangeExpectation `yaml:"file_change"`
	Command    CommandExpectation    `yaml:"command"`
	Approvals  []ApprovalExpectation `yaml:"approvals"`
}

// TextExpectation summarizes a text-producing message category.
type TextExpectation struct {
	NumMessages int    `yaml:"num_messages"`
	Content     string `yaml:"content"`
}

// ReasoningExpectation summarizes reasoning messages and stream lifecycle.
type ReasoningExpectation struct {
	NumMessages int    `yaml:"num_messages"`
	Content     string `yaml:"content"`
	NumStreams  int    `yaml:"num_streams"`
	AllFlushed  bool   `yaml:"all_flushed"`
}

// FileChangeExpectation summarizes file-change messages observed in a turn.
type FileChangeExpectation struct {
	NumMessages int      `yaml:"num_messages"`
	Files       []string `yaml:"files"`
}

// CommandExpectation summarizes command messages observed in a turn.
type CommandExpectation struct {
	NumMessages int      `yaml:"num_messages"`
	Executed    []string `yaml:"executed"`
}

// ApprovalExpectation records one normalized approval request/decision pair.
type ApprovalExpectation struct {
	Kind     agent.ApprovalKind   `yaml:"kind"`
	Decision agent.ApprovalOption `yaml:"decision"`
}

// Step is one replayable protocol action in the transcript script.
type Step struct {
	Kind    string `json:"kind"`
	Message any    `json:"message,omitempty"`
	Match   any    `json:"match,omitempty"`
	DelayMS int    `json:"delay_ms,omitempty"`
}
