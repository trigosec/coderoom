package queue

import (
	"testing"
	"time"
)

func TestQueue_deliversInOrder(t *testing.T) {
	q := New[int]()
	t.Cleanup(q.Close)
	want := []int{1, 2, 3}
	go func() {
		for _, v := range want {
			q.Push(v)
		}
	}()
	for _, w := range want {
		got, ok := q.Pull()
		if !ok {
			t.Fatal("queue closed unexpectedly")
		}
		if got != w {
			t.Errorf("got %d, want %d", got, w)
		}
	}
}

func TestQueue_concurrentProducers(_ *testing.T) {
	q := New[int]()
	defer q.Close()
	const n = 50
	// Two goroutines push concurrently; all values must arrive.
	for range 2 {
		go func() {
			for range n {
				q.Push(1)
			}
		}()
	}
	for range 2 * n {
		q.Pull()
	}
}

func TestQueue_closeUnblocksPull(t *testing.T) {
	q := New[int]()
	done := make(chan struct{})
	go func() {
		if _, ok := q.Pull(); ok {
			t.Error("expected Pull to report closed")
		}
		close(done)
	}()
	q.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Pull to unblock after Close")
	}
}

func TestQueue_pushAfterCloseDoesNotBlock(t *testing.T) {
	q := New[int]()
	q.Close()
	done := make(chan struct{})
	go func() {
		q.Push(1)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Push blocked after Close")
	}
}

func TestQueue_tryPull(t *testing.T) {
	q := New[int]()
	t.Cleanup(q.Close)
	if _, ok := q.TryPull(); ok {
		t.Fatal("expected no value available")
	}
	q.Push(7)
	deadline := time.After(time.Second)
	for {
		if v, ok := q.TryPull(); ok {
			if v != 7 {
				t.Errorf("got %d, want 7", v)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pushed value")
		default:
		}
	}
}

func TestQueue_pullTimeout(t *testing.T) {
	q := New[int]()
	t.Cleanup(q.Close)
	if _, ok := q.PullTimeout(10 * time.Millisecond); ok {
		t.Fatal("expected timeout with no value available")
	}
	q.Push(9)
	v, ok := q.PullTimeout(time.Second)
	if !ok {
		t.Fatal("expected a value before the timeout")
	}
	if v != 9 {
		t.Errorf("got %d, want 9", v)
	}
}
