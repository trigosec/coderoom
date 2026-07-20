package promptlang_test

import (
	"testing"

	"github.com/trigosec/coderoom/internal/promptlang"
)

func TestParse_slashCommands(t *testing.T) {
	tests := []struct {
		input string
		want  promptlang.Statement
	}{
		{"/invite ada", promptlang.Invite{Alias: "ada"}},
		{"/invite   ada  ", promptlang.Invite{Alias: "ada"}},
		{"/remove ada", promptlang.Remove{Alias: "ada"}},
		{"/cancel ada", promptlang.Cancel{Alias: "ada"}},
		{"/handoff ada turing", promptlang.Handoff{FromAlias: "ada", ToAlias: "turing"}},
		{"/who", promptlang.Who{}},
		{"/help", promptlang.Help{}},
		{"/quit", promptlang.Quit{}},
	}
	for _, tt := range tests {
		got, err := promptlang.Parse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Parse(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParse_sendAction(t *testing.T) {
	got, err := promptlang.Parse("@ada do the thing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := promptlang.Send{Alias: "ada", Text: "do the thing"}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParse_broadcast(t *testing.T) {
	got, err := promptlang.Parse("hello everyone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != (promptlang.Broadcast{Text: "hello everyone"}) {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestParse_trimming(t *testing.T) {
	got, err := promptlang.Parse("  /invite ada  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != (promptlang.Invite{Alias: "ada"}) {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestParse_errors(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"/invite"},
		{"/invite   "},
		{"/remove"},
		{"/remove   "},
		{"/cancel"},
		{"/cancel   "},
		{"/handoff"},
		{"/handoff ada"},
		{"/handoff ada turing extra"},
		{"@ada"},
		{"@ada   "},
		{"@ ada hi"}, // space between @ and alias
		{""},         // empty
		{"   "},      // whitespace only
		{"/unknown"},
	}
	for _, tt := range tests {
		_, err := promptlang.Parse(tt.input)
		if err == nil {
			t.Errorf("Parse(%q): expected error, got nil", tt.input)
		}
	}
}
