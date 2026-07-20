package ui

import (
	"context"
	"sync"
)

type executionLifetime struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

func newExecutionLifetime(parent context.Context) *executionLifetime {
	ctx, cancel := context.WithCancel(parent)
	return &executionLifetime{ctx: ctx, cancel: cancel}
}

func (l *executionLifetime) start() (context.Context, func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, nil, context.Canceled
	}
	l.wg.Add(1)
	return l.ctx, l.wg.Done, nil
}

func (l *executionLifetime) cancelActive() {
	l.cancel()
}

func (l *executionLifetime) close() {
	l.mu.Lock()
	l.closed = true
	l.cancel()
	l.mu.Unlock()
	l.wg.Wait()
}
