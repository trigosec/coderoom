// Package queue provides a generic unbounded FIFO that decouples a producer
// goroutine from a consumer pull loop.
package queue

import (
	"sync"
	"time"
)

// Queue is an unbounded, ordered FIFO. Push returns as soon as the value is
// queued, without waiting for a Pull. All concurrency — the pump goroutine,
// internal buffer, and channels — is owned and managed here.
type Queue[T any] struct {
	in        chan T
	out       chan T
	done      chan struct{}
	closeOnce sync.Once
}

// New returns a Queue with its pump goroutine already running.
func New[T any]() *Queue[T] {
	q := &Queue[T]{
		in:   make(chan T),
		out:  make(chan T),
		done: make(chan struct{}),
	}
	go q.pump()
	return q
}

// Close stops the queue's pump goroutine. After Close, Push is a no-op and
// Pull/TryPull/PullTimeout report no value, discarding anything still
// buffered. Safe to call more than once.
func (q *Queue[T]) Close() {
	q.closeOnce.Do(func() {
		close(q.done)
	})
}

// Push appends a value to the queue. A no-op if the queue has been closed.
func (q *Queue[T]) Push(v T) {
	select {
	case q.in <- v:
	case <-q.done:
	}
}

// Pull blocks until the next value is available or the queue is closed.
// ok is false only when the queue has been closed.
func (q *Queue[T]) Pull() (v T, ok bool) {
	select {
	case v = <-q.out:
		return v, true
	case <-q.done:
		return v, false
	}
}

// TryPull returns the next value if one is immediately available.
func (q *Queue[T]) TryPull() (T, bool) {
	select {
	case v := <-q.out:
		return v, true
	default:
		var zero T
		return zero, false
	}
}

// PullTimeout waits up to d for the next value.
func (q *Queue[T]) PullTimeout(d time.Duration) (T, bool) {
	select {
	case v := <-q.out:
		return v, true
	case <-time.After(d):
		var zero T
		return zero, false
	case <-q.done:
		var zero T
		return zero, false
	}
}

// pump bridges in to out via an internal slice, so Push never blocks on Pull.
func (q *Queue[T]) pump() {
	var buf []T
	for {
		if len(buf) == 0 {
			select {
			case v := <-q.in:
				buf = append(buf, v)
			case <-q.done:
				return
			}
			continue
		}
		select {
		case v := <-q.in:
			buf = append(buf, v)
		case q.out <- buf[0]:
			buf = buf[1:]
		case <-q.done:
			return
		}
	}
}
