package interpreter

import (
	"errors"
	"testing"

	"github.com/trigosec/coderoom/internal/session"
)

func TestClose_flushesPublishedEventsThroughObservers(t *testing.T) {
	interp, _ := newSubmitContractInterpreterWithoutCleanup(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	delivered := make(chan Event, 2)
	interp.AddObserver(blockingEventObserver{
		entered:   entered,
		release:   release,
		delivered: delivered,
	})

	interp.publish(UnknownCommand{Raw: "/not-defined", Name: "not-defined"})
	receiveSignal(t, entered, "first event delivery")
	interp.publish(InputRejected{Raw: "/invite", Err: errors.New("invalid input")})

	closed := make(chan struct{})
	go func() {
		interp.Close()
		close(closed)
	}()
	assertNoSignal(t, closed, "Close returned before published events were delivered")
	close(release)
	receiveSignal(t, closed, "Close")

	if _, ok := (<-delivered).(UnknownCommand); !ok {
		t.Fatal("first event was not UnknownCommand")
	}
	if _, ok := (<-delivered).(InputRejected); !ok {
		t.Fatal("second event was not InputRejected")
	}
}

func TestClose_flushesAcceptedSubmissionOutcome(t *testing.T) {
	interp, sess := newSubmitContractInterpreterWithoutCleanup(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	delivered := make(chan Event, 3)
	interp.AddObserver(blockingAcceptedObserver{
		entered:   entered,
		release:   release,
		delivered: delivered,
	})

	mustSubmit(t, interp.SubmitWithFallback("/cancel ada", session.CancelCommand{Alias: "ada"}))
	receiveSignal(t, entered, "input acceptance delivery")
	receiveSubmitCommand(t, sess.executed)
	closed := make(chan struct{})
	go func() {
		interp.Close()
		close(closed)
	}()
	assertNoSignal(t, closed, "Close returned before submission outcome was delivered")
	close(release)
	receiveSignal(t, closed, "Close")

	if _, ok := (<-delivered).(InputAccepted); !ok {
		t.Fatal("first event was not InputAccepted")
	}
	if _, ok := (<-delivered).(StateChanged); !ok {
		t.Fatal("second event was not StateChanged")
	}
	if _, ok := (<-delivered).(SubmissionSucceeded); !ok {
		t.Fatal("terminal event was not SubmissionSucceeded")
	}
}

type blockingEventObserver struct {
	entered   chan struct{}
	release   chan struct{}
	delivered chan Event
}

type blockingAcceptedObserver struct {
	entered   chan struct{}
	release   chan struct{}
	delivered chan Event
}

func (o blockingAcceptedObserver) OnEvent(event Event) {
	if _, accepted := event.(InputAccepted); accepted {
		close(o.entered)
		<-o.release
	}
	o.delivered <- event
}

func (o blockingEventObserver) OnEvent(event Event) {
	if _, unknown := event.(UnknownCommand); unknown {
		close(o.entered)
		<-o.release
	}
	o.delivered <- event
}

func newSubmitContractInterpreterWithoutCleanup(t *testing.T) (*Interpreter, *submitContractSession) {
	t.Helper()
	sess := newSubmitContractSession()
	interp := New(t.Context(), sess, t.TempDir())
	return interp, sess
}
