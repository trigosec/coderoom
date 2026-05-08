package ui

import "github.com/trigosec/coderoom/internal/session"

// eventQueue is an unbounded, ordered FIFO that bridges session.Observer calls
// (on agent reader goroutines) to the Bubble Tea event loop (on the main goroutine).
// All concurrency — the pump goroutine, internal buffer, and channels — is owned
// and managed here. Callers interact only through Push and out.
type eventQueue struct {
	in  chan session.Event
	out chan session.Event
}

func newEventQueue() *eventQueue {
	q := &eventQueue{
		in:  make(chan session.Event),
		out: make(chan session.Event),
	}
	go q.pump()
	return q
}

// Push appends an event to the queue. It decouples producers from the consumer
// via an internal buffer: Push returns as soon as the pump receives the event,
// without waiting for Pull to be called. If the consumer stops pulling,
// backpressure propagates through the pump and Push will eventually block.
func (q *eventQueue) Push(e session.Event) {
	q.in <- e
}

// Pull blocks until the next event is available and returns it.
// Returns false if the queue has been closed.
func (q *eventQueue) Pull() (session.Event, bool) {
	e, ok := <-q.out
	return e, ok
}

// pump bridges in to out via an internal slice
func (q *eventQueue) pump() {
	var buf []session.Event
	for {
		if len(buf) == 0 {
			e, ok := <-q.in
			if !ok {
				close(q.out)
				return
			}
			buf = append(buf, e)
		} else {
			select {
			case e, ok := <-q.in:
				if !ok {
					for _, pending := range buf {
						q.out <- pending
					}
					close(q.out)
					return
				}
				buf = append(buf, e)
			case q.out <- buf[0]:
				buf = buf[1:]
			}
		}
	}
}
