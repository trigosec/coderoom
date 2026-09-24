package interpreter_test

import (
	"context"
	"errors"
	"testing"

	"github.com/trigosec/coderoom/internal/interpreter"
	"github.com/trigosec/coderoom/internal/promptlang"
)

func TestInterpreter_ownsRoomScopedCommandDefinitions(t *testing.T) {
	first := interpreter.New(context.Background(), newRecordingSession(), t.TempDir())
	t.Cleanup(first.Close)
	second := interpreter.New(context.Background(), newRecordingSession(), t.TempDir())
	t.Cleanup(second.Close)
	definition := promptlang.CommandDefinition{
		Name: "tests",
		Body: promptlang.Shell{Program: "go test ./..."},
	}

	if err := first.DefineCommand(definition); err != nil {
		t.Fatalf("DefineCommand: %v", err)
	}
	body, err := first.ResolveCommand(promptlang.CommandInvocation{Name: "tests"})
	if err != nil {
		t.Fatalf("ResolveCommand: %v", err)
	}
	if body != definition.Body {
		t.Fatalf("body = %#v, want %#v", body, definition.Body)
	}
	if _, err := second.ResolveCommand(promptlang.CommandInvocation{Name: "tests"}); err == nil {
		t.Fatal("second interpreter resolved command defined in first interpreter")
	}
}

func TestInterpreter_commandRegistryRejectsAfterShutdown(t *testing.T) {
	interp := interpreter.New(context.Background(), newRecordingSession(), t.TempDir())
	interp.Close()

	definition := promptlang.CommandDefinition{Name: "tests", Body: promptlang.Shell{Program: "true"}}
	if err := interp.DefineCommand(definition); !errors.Is(err, interpreter.ErrClosed) {
		t.Fatalf("DefineCommand error = %v, want ErrClosed", err)
	}
	if _, err := interp.ResolveCommand(promptlang.CommandInvocation{Name: "tests"}); !errors.Is(err, interpreter.ErrClosed) {
		t.Fatalf("ResolveCommand error = %v, want ErrClosed", err)
	}
}
