package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type participantConfigTest struct {
	name    string
	files   map[string]string
	alias   string
	want    ParticipantConfig
	wantErr error
}

var successParticipantConfigTests = []participantConfigTest{
	{
		name:  "fallback to identification only when yaml is missing",
		alias: "ada",
		want: ParticipantConfig{
			Alias:          "ada",
			Prompt:         "Your name is ada.\nYou will be referred to as ada or @ada.",
			LoadedFromDisk: false,
		},
	},
	{
		name:  "load participant role and prompt from disk",
		alias: "ada",
		files: map[string]string{
			".coderoom/participants/ada.yaml":     "alias: ada\nrole: reviewer\n",
			".coderoom/prompts/roles/reviewer.md": "Focus on correctness.",
		},
		want: ParticipantConfig{
			Alias:          "ada",
			Role:           "reviewer",
			Prompt:         "Your name is ada.\nYou will be referred to as ada or @ada.\n\nYou are a reviewer.\n\nFocus on correctness.",
			LoadedFromDisk: true,
		},
	},
	{
		name:  "load participant without role",
		alias: "ada",
		files: map[string]string{
			".coderoom/participants/ada.yaml": "alias: ada\n",
		},
		want: ParticipantConfig{
			Alias:          "ada",
			Prompt:         "Your name is ada.\nYou will be referred to as ada or @ada.",
			LoadedFromDisk: true,
		},
	},
	{
		name:  "accept empty role when present",
		alias: "ada",
		files: map[string]string{
			".coderoom/participants/ada.yaml": "alias: ada\nrole: \"\"\n",
		},
		want: ParticipantConfig{
			Alias:          "ada",
			Prompt:         "Your name is ada.\nYou will be referred to as ada or @ada.",
			LoadedFromDisk: true,
		},
	},
	{
		name:  "accept null role when present",
		alias: "ada",
		files: map[string]string{
			".coderoom/participants/ada.yaml": "alias: ada\nrole: null\n",
		},
		want: ParticipantConfig{
			Alias:          "ada",
			Prompt:         "Your name is ada.\nYou will be referred to as ada or @ada.",
			LoadedFromDisk: true,
		},
	},
}

var errorParticipantConfigTests = []participantConfigTest{
	{
		name:  "reject malformed yaml",
		alias: "ada",
		files: map[string]string{
			".coderoom/participants/ada.yaml": "alias: [ada\n",
		},
		wantErr: errInvalidParticipantYAML,
	},
	{
		name:  "reject empty alias in yaml",
		alias: "ada",
		files: map[string]string{
			".coderoom/participants/ada.yaml": "alias:\n",
		},
		wantErr: errInvalidParticipantYAML,
	},
	{
		name:  "reject alias mismatch",
		alias: "ada",
		files: map[string]string{
			".coderoom/participants/ada.yaml": "alias: turing\n",
		},
		wantErr: errInvalidParticipantYAML,
	},
	{
		name:  "reject missing role prompt",
		alias: "ada",
		files: map[string]string{
			".coderoom/participants/ada.yaml": "alias: ada\nrole: reviewer\n",
		},
		wantErr: errRolePromptNotFound,
	},
	{
		name:    "reject empty requested alias",
		alias:   "  ",
		wantErr: errInvalidParticipantAlias,
	},
	{
		name:    "reject alias path traversal",
		alias:   "../../secrets",
		wantErr: errInvalidParticipantAlias,
	},
	{
		name:  "reject role path traversal",
		alias: "ada",
		files: map[string]string{
			".coderoom/participants/ada.yaml": "alias: ada\nrole: ../../secrets\n",
		},
		wantErr: errInvalidParticipantRole,
	},
	{
		name:  "reject multiple yaml documents",
		alias: "ada",
		files: map[string]string{
			".coderoom/participants/ada.yaml": "alias: ada\n---\nrole: reviewer\n",
		},
		wantErr: errInvalidParticipantYAML,
	},
}

func TestForParticipantSuccess(t *testing.T) {
	t.Parallel()
	runParticipantConfigTests(t, successParticipantConfigTests)
}

func TestForParticipantErrors(t *testing.T) {
	t.Parallel()
	runParticipantConfigTests(t, errorParticipantConfigTests)
}

func runParticipantConfigTests(t *testing.T, tests []participantConfigTest) {
	t.Helper()

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runParticipantConfigTest(t, tt)
		})
	}
}

func runParticipantConfigTest(t *testing.T, tt participantConfigTest) {
	t.Helper()

	root := t.TempDir()
	writeFiles(t, root, tt.files)

	cfg := New(root)
	got, err := cfg.ForParticipant(tt.alias)
	if tt.wantErr != nil {
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, tt.wantErr) {
			t.Fatalf("expected error matching %v, got %v", tt.wantErr, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("ForParticipant(%q): %v", tt.alias, err)
	}
	if got != tt.want {
		t.Fatalf("ForParticipant(%q) = %+v, want %+v", tt.alias, got, tt.want)
	}
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("MkdirAll(%q): %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%q): %v", path, err)
		}
	}
}
