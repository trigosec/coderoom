package ui

import (
	"testing"

	"github.com/trigosec/coderoom/internal/session"
)

func TestEventQueue_deliversInOrder(t *testing.T) {
	q := newEventQueue()
	want := []session.Event{
		{Kind: session.KindAgentStarted, Alias: "ada"},
		{Kind: session.KindDelta, Text: "hello"},
		{Kind: session.KindDone, Alias: "ada"},
	}
	go func() {
		for _, e := range want {
			q.Push(e)
		}
	}()
	for _, w := range want {
		got, ok := q.Pull()
		if !ok {
			t.Fatal("queue closed unexpectedly")
		}
		if got != w {
			t.Errorf("got %+v, want %+v", got, w)
		}
	}
}

func TestEventQueue_concurrentProducers(t *testing.T) {
	q := newEventQueue()
	const n = 50
	// Two goroutines push concurrently; all events must arrive.
	for range 2 {
		go func() {
			for range n {
				q.Push(session.Event{Kind: session.KindDelta})
			}
		}()
	}
	for range 2 * n {
		if _, ok := q.Pull(); !ok {
			t.Fatal("queue closed unexpectedly")
		}
	}
}
