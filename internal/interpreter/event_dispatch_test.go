package interpreter

import (
	"errors"
	"testing"
)

func TestClose_flushesPublishedEventsThroughObservers(t *testing.T) {
	interp, _, _ := newSubmitContractInterpreterWithoutCleanup(t)
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

type blockingEventObserver struct {
	entered   chan struct{}
	release   chan struct{}
	delivered chan Event
}

func (o blockingEventObserver) OnEvent(event Event) {
	if _, unknown := event.(UnknownCommand); unknown {
		close(o.entered)
		<-o.release
	}
	o.delivered <- event
}

func newSubmitContractInterpreterWithoutCleanup(t *testing.T) (*Interpreter, *submitContractSession, chan Event) {
	t.Helper()
	sess := newSubmitContractSession()
	interp := New(t.Context(), sess, t.TempDir())
	events := make(chan Event, 64)
	interp.AddObserver(submitContractObserver{events: events})
	return interp, sess, events
}
