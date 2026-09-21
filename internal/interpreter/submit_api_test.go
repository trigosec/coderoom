package interpreter_test

import (
	"context"
	"testing"

	"github.com/trigosec/coderoom/internal/interpreter"
	"github.com/trigosec/coderoom/internal/session"
)

func TestSubmitAPI_reportsUnknownCommandWithoutFallback(t *testing.T) {
	interp, _, events := newSubmitExample()
	defer interp.Close()

	interp.Submit("/not-defined")

	unknown := receiveEvent[interpreter.UnknownCommand](t, events)
	if unknown.Raw != "/not-defined" || unknown.Name != "not-defined" {
		t.Fatalf("unknown command = %#v", unknown)
	}
}

func TestSubmitAPI_rejectsInvalidArguments(t *testing.T) {
	interp, _, events := newSubmitExample()
	defer interp.Close()

	interp.Submit("/invite")

	rejected := receiveEvent[interpreter.InputRejected](t, events)
	if rejected.Raw != "/invite" || rejected.Err == nil {
		t.Fatalf("rejected input = %#v", rejected)
	}
}

func TestSubmitAPI_executesMigrationFallback(t *testing.T) {
	interp, sess, events := newSubmitExample()
	defer interp.Close()

	interp.SubmitWithFallback(
		"/cancel ada",
		session.CancelCommand{Alias: "ada"},
	)

	accepted := receiveEvent[interpreter.InputAccepted](t, events)
	if accepted.Raw != "/cancel ada" {
		t.Fatalf("accepted input = %#v", accepted)
	}
	command := receiveCommand(t, sess.executed)
	want := session.CancelCommand{Alias: "ada"}
	if command != want {
		t.Fatalf("fallback command = %#v, want %#v", command, want)
	}
}

func newSubmitExample() (*interpreter.Interpreter, *recordingSession, chan interpreter.Event) {
	sess := newRecordingSession()
	interp := interpreter.New(context.Background(), sess, ".")
	events := make(chan interpreter.Event, 8)
	interp.AddObserver(eventObserver{events: events})
	return interp, sess, events
}
