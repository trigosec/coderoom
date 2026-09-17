// Package interpreter implements coderoom's UI-independent application layer.
package interpreter

import (
	"context"
	"sync"

	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/queue"
	"github.com/trigosec/coderoom/internal/room"
	"github.com/trigosec/coderoom/internal/session"
)

// Option configures an Interpreter.
type Option func(*Interpreter)

type operation interface{ apply(*Interpreter) }

type sessionEventOperation struct{ event session.Event }
type snapshotOperation struct{ result chan Snapshot }
type resolveApprovalOperation struct {
	id     int64
	choice ApprovalChoice
}
type shutdownOperation struct{}

// Interpreter serializes application operations and session dispatch.
type Interpreter struct {
	session SessionController
	room    *room.Room

	operations   *queue.Queue[operation]
	events       *queue.Queue[Event]
	done         chan struct{}
	dispatchDone chan struct{}
	cancel       context.CancelFunc
	closeOnce    sync.Once
	enqueueMu    sync.Mutex
	closed       bool

	observerMu sync.RWMutex
	observers  []Observer
	stateMu    sync.RWMutex
	approval   *Approval
}

// New starts an Interpreter backed by sess.
func New(ctx context.Context, sess SessionController, _ string, opts ...Option) *Interpreter {
	if ctx == nil {
		ctx = context.Background()
	}
	lifetime, cancel := context.WithCancel(ctx)
	i := &Interpreter{
		session:      sess,
		room:         room.New(),
		operations:   queue.New[operation](),
		events:       queue.New[Event](),
		done:         make(chan struct{}),
		dispatchDone: make(chan struct{}),
		cancel:       cancel,
	}
	for _, opt := range opts {
		opt(i)
	}
	sess.AddObserver(sessionObserver{interpreter: i})
	go i.run()
	go i.dispatchEvents()
	go func() {
		select {
		case <-lifetime.Done():
			i.requestClose()
		case <-i.done:
		}
	}()
	return i
}

// ResolveApproval queues a structured response to the active approval.
func (i *Interpreter) ResolveApproval(id int64, choice ApprovalChoice) {
	if !i.enqueue(resolveApprovalOperation{id: id, choice: choice}) {
		return
	}
}

// Snapshot returns a detached view of current application state.
func (i *Interpreter) Snapshot() Snapshot {
	result := make(chan Snapshot, 1)
	if !i.enqueue(snapshotOperation{result: result}) {
		return i.captureSnapshot()
	}
	select {
	case snapshot := <-result:
		return snapshot
	case <-i.done:
		return i.captureSnapshot()
	}
}

// AddObserver registers an application event observer.
func (i *Interpreter) AddObserver(observer Observer) {
	if observer == nil {
		return
	}
	i.observerMu.Lock()
	defer i.observerMu.Unlock()
	i.observers = append(i.observers, observer)
}

// Close stops the interpreter and its owned background work.
func (i *Interpreter) Close() {
	i.requestClose()
	<-i.done
	<-i.dispatchDone
}

func (i *Interpreter) requestClose() {
	i.closeOnce.Do(func() {
		i.enqueueMu.Lock()
		i.closed = true
		i.operations.Push(shutdownOperation{})
		i.enqueueMu.Unlock()
	})
}

func (i *Interpreter) enqueue(op operation) bool {
	i.enqueueMu.Lock()
	defer i.enqueueMu.Unlock()
	if i.closed {
		return false
	}
	i.operations.Push(op)
	return true
}

func (i *Interpreter) run() {
	for {
		op, ok := i.operations.Pull()
		if !ok {
			return
		}
		op.apply(i)
		if _, stopping := op.(shutdownOperation); stopping {
			return
		}
	}
}

func (op sessionEventOperation) apply(i *Interpreter) {
	i.room.ApplyEvent(op.event)
	switch event := op.event.(type) {
	case session.ApprovalRequested:
		approval := approvalFromAgent(event.ID, event.Alias, event.Req)
		i.stateMu.Lock()
		i.approval = &approval
		i.stateMu.Unlock()
	case session.ApprovalCleared:
		if !i.clearApproval(event.ID) {
			return
		}
	}
	i.publish(StateChanged{Snapshot: i.captureSnapshot()})
}

func (op snapshotOperation) apply(i *Interpreter) {
	op.result <- i.captureSnapshot()
}

func (op resolveApprovalOperation) apply(i *Interpreter) {
	choice, err := i.approvalChoice(op.id, op.choice)
	if err != nil {
		i.publish(OperationFailed{Operation: "resolve approval", Err: err})
		return
	}
	err = i.session.Execute(session.ResolveApprovalCommand{ApprovalID: op.id, Choice: choice})
	if err != nil {
		i.publish(OperationFailed{Operation: "resolve approval", Err: err})
		return
	}
	i.clearApproval(op.id)
	i.publish(StateChanged{Snapshot: i.captureSnapshot()})
}

func (shutdownOperation) apply(i *Interpreter) {
	i.session.Shutdown()
	i.cancel()
	i.operations.Close()
	i.room.Close()
	i.events.Close()
	close(i.done)
}

func (i *Interpreter) captureSnapshot() Snapshot {
	snapshot := Snapshot{
		Room:         i.room.Snapshot(),
		Participants: append([]participant.View(nil), i.session.Roster()...),
	}
	i.stateMu.RLock()
	defer i.stateMu.RUnlock()
	if i.approval != nil {
		approval := *i.approval
		approval.Options = append([]ApprovalOption(nil), approval.Options...)
		snapshot.Approval = &approval
	}
	return snapshot
}

func (i *Interpreter) publish(event Event) {
	i.events.Push(event)
}

func (i *Interpreter) dispatchEvents() {
	defer close(i.dispatchDone)
	for {
		event, ok := i.events.Pull()
		if !ok {
			return
		}
		i.observerMu.RLock()
		observers := append([]Observer(nil), i.observers...)
		i.observerMu.RUnlock()
		for _, observer := range observers {
			observer.OnEvent(event)
		}
	}
}
