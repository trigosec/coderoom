package ui_test

import (
	"testing"

	"github.com/trigosec/coderoom/internal/ui"
)

func TestParse_slashCommands(t *testing.T) {
	tests := []struct {
		input string
		want  ui.Action
	}{
		{"/invite ada", ui.Invite{Alias: "ada"}},
		{"/invite   ada  ", ui.Invite{Alias: "ada"}},
		{"/remove ada", ui.Remove{Alias: "ada"}},
		{"/cancel ada", ui.Cancel{Alias: "ada"}},
		{"/who", ui.Who{}},
		{"/help", ui.Help{}},
		{"/quit", ui.Quit{}},
	}
	for _, tt := range tests {
		got, err := ui.Parse(tt.input)
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
	got, err := ui.Parse("@ada do the thing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := ui.Send{Alias: "ada", Text: "do the thing"}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParse_broadcast(t *testing.T) {
	got, err := ui.Parse("hello everyone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != (ui.Broadcast{Text: "hello everyone"}) {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestParse_trimming(t *testing.T) {
	got, err := ui.Parse("  /invite ada  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != (ui.Invite{Alias: "ada"}) {
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
		{"@ada"},
		{"@ada   "},
		{"@ ada hi"}, // space between @ and alias
		{""},         // empty
		{"   "},      // whitespace only
		{"/unknown"},
	}
	for _, tt := range tests {
		_, err := ui.Parse(tt.input)
		if err == nil {
			t.Errorf("Parse(%q): expected error, got nil", tt.input)
		}
	}
}
