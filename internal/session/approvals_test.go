package session

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
)

func TestApprovalHub_RequestEmitsEventAndBlocksUntilResolved(t *testing.T) {
	events := make(chan Event, 1)
	h := newApprovalHub(func(e Event) { events <- e })
	l := h.Listener("alice")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := startApprovalDecision(ctx, t, l, "approve?")
	ev := mustApprovalEvent(ctx, t, events)
	assertApprovalRequestEvent(t, ev, "alice", "approve?")

	if !h.resolve(ev.ApprovalID, agent.OptionAccept) {
		t.Fatal("expected resolve to succeed")
	}

	assertDecisionChoice(t, mustDecision(ctx, t, done), agent.OptionAccept)
}

func TestApprovalHub_QueuesRequestsFIFO(t *testing.T) {
	events := make(chan Event, 4)
	h := newApprovalHub(func(e Event) { events <- e })
	listener := h.Listener("alice")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	firstDecision := startApprovalDecision(ctx, t, listener, "first?")
	first := mustApprovalEvent(ctx, t, events)
	assertApprovalAsk(t, first, "first?")

	secondDecision := startApprovalDecision(ctx, t, listener, "second?")
	assertNoApprovalEvent(t, events, 100*time.Millisecond)

	if !h.resolve(first.ApprovalID, agent.OptionAccept) {
		t.Fatal("expected first resolve to succeed")
	}

	second := mustApprovalEvent(ctx, t, events)
	assertApprovalAsk(t, second, "second?")

	if !h.resolve(second.ApprovalID, agent.OptionDecline) {
		t.Fatal("expected second resolve to succeed")
	}

	gotChoices := make(map[agent.ApprovalOption]int, 2)
	gotChoices[mustDecision(ctx, t, firstDecision).Choice]++
	gotChoices[mustDecision(ctx, t, secondDecision).Choice]++
	if gotChoices[agent.OptionAccept] != 1 {
		t.Fatalf("accept decisions = %d, want 1", gotChoices[agent.OptionAccept])
	}
	if gotChoices[agent.OptionDecline] != 1 {
		t.Fatalf("decline decisions = %d, want 1", gotChoices[agent.OptionDecline])
	}
}

func TestApprovalHub_CancelActiveRequestPublishesClearedEvent(t *testing.T) {
	events := make(chan Event, 2)
	h := newApprovalHub(func(e Event) { events <- e })
	listener := h.Listener("alice")
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := listener.Decide(ctx, agent.ApprovalRequest{
			Kind:    agent.ApprovalCommandExecution,
			Ask:     "approve?",
			Options: []agent.ApprovalOption{agent.OptionAccept, agent.OptionDecline},
		})
		done <- err
	}()

	ev := mustApprovalEvent(waitCtx, t, events)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected canceled approval to return an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for canceled approval to return")
	}

	cleared := mustEventKind(waitCtx, t, events, KindApprovalCleared)
	if cleared.ApprovalID != ev.ApprovalID {
		t.Fatalf("cleared approval id = %d, want %d", cleared.ApprovalID, ev.ApprovalID)
	}
}

func TestApprovalHub_CancelActiveRequestPublishesNextQueuedApproval(t *testing.T) {
	events := make(chan Event, 4)
	h := newApprovalHub(func(e Event) { events <- e })
	listener := h.Listener("alice")

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	secondCtx, cancelSecond := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelSecond()

	startCancelableApproval(firstCtx, listener, "first?")
	first := mustApprovalEvent(secondCtx, t, events)
	done := startApprovalDecision(secondCtx, t, listener, "second?")

	assertNoApprovalEvent(t, events, 100*time.Millisecond)
	cancelFirst()

	second := mustApprovalEvent(secondCtx, t, events)
	assertApprovalRequestEvent(t, second, "alice", "second?")
	if second.ApprovalID == first.ApprovalID {
		t.Fatal("expected queued approval to have a different approval id")
	}

	if !h.resolve(second.ApprovalID, agent.OptionAccept) {
		t.Fatal("expected second resolve to succeed")
	}

	assertDecisionChoice(t, mustDecision(secondCtx, t, done), agent.OptionAccept)
}

func TestApprovalHub_ConcurrentRequestsPublishIncreasingApprovalIDs(t *testing.T) {
	const total = 24

	events := make(chan Event, total)
	h := newApprovalHub(func(e Event) { events <- e })
	listener := h.Listener("alice")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := listener.Decide(ctx, approvalRequest(fmt.Sprintf("approval-%02d", i)))
			if err != nil && ctx.Err() == nil {
				t.Errorf("Decide(%d) returned error: %v", i, err)
			}
		}(i)
	}
	close(start)

	var gotIDs []int64
	for i := 0; i < total; i++ {
		ev := mustApprovalEvent(ctx, t, events)
		gotIDs = append(gotIDs, ev.ApprovalID)
		if !h.resolve(ev.ApprovalID, agent.OptionDecline) {
			t.Fatalf("expected resolve(%d) to succeed", ev.ApprovalID)
		}
	}

	wg.Wait()
	for i := 1; i < len(gotIDs); i++ {
		if gotIDs[i-1] >= gotIDs[i] {
			t.Fatalf("approval ids not strictly increasing: %v", gotIDs)
		}
	}
}

func startApprovalDecision(ctx context.Context, t *testing.T, listener agent.ApprovalListener, ask string) <-chan agent.ApprovalDecision {
	t.Helper()
	done := make(chan agent.ApprovalDecision, 1)
	go func() {
		d, err := listener.Decide(ctx, approvalRequest(ask))
		if err != nil {
			t.Errorf("Decide(%q) returned error: %v", ask, err)
			return
		}
		done <- d
	}()
	return done
}

func startCancelableApproval(ctx context.Context, listener agent.ApprovalListener, ask string) {
	go func() {
		_, _ = listener.Decide(ctx, approvalRequest(ask))
	}()
}

func approvalRequest(ask string) agent.ApprovalRequest {
	return agent.ApprovalRequest{
		Kind:    agent.ApprovalCommandExecution,
		Ask:     ask,
		Options: []agent.ApprovalOption{agent.OptionAccept, agent.OptionDecline},
	}
}

func assertApprovalRequestEvent(t *testing.T, ev Event, wantAlias, wantAsk string) {
	t.Helper()
	if ev.Kind != KindApprovalRequested {
		t.Fatalf("event kind = %q, want %q", ev.Kind, KindApprovalRequested)
	}
	if ev.Alias != wantAlias {
		t.Fatalf("event alias = %q, want %q", ev.Alias, wantAlias)
	}
	if ev.ApprovalID == 0 {
		t.Fatal("expected non-zero approval id")
	}
	assertApprovalAsk(t, ev, wantAsk)
}

func assertApprovalAsk(t *testing.T, ev Event, want string) {
	t.Helper()
	if ev.ApprovalReq == nil || ev.ApprovalReq.Ask != want {
		t.Fatalf("approval req = %#v, want ask %q", ev.ApprovalReq, want)
	}
}

func assertDecisionChoice(t *testing.T, got agent.ApprovalDecision, want agent.ApprovalOption) {
	t.Helper()
	if got.Choice != want {
		t.Fatalf("choice = %q, want %q", got.Choice, want)
	}
}

func mustApprovalEvent(ctx context.Context, t *testing.T, events <-chan Event) Event {
	t.Helper()
	select {
	case ev := <-events:
		return ev
	case <-ctx.Done():
		t.Fatal("timed out waiting for approval requested event")
		return Event{}
	}
}

func mustEventKind(ctx context.Context, t *testing.T, events <-chan Event, want Kind) Event {
	t.Helper()
	ev := mustApprovalEvent(ctx, t, events)
	if ev.Kind != want {
		t.Fatalf("event kind = %q, want %q", ev.Kind, want)
	}
	return ev
}

func assertNoApprovalEvent(t *testing.T, events <-chan Event, wait time.Duration) {
	t.Helper()
	select {
	case ev := <-events:
		t.Fatalf("unexpected approval event: %#v", ev)
	case <-time.After(wait):
	}
}

func mustDecision(ctx context.Context, t *testing.T, decisions <-chan agent.ApprovalDecision) agent.ApprovalDecision {
	t.Helper()
	select {
	case d := <-decisions:
		return d
	case <-ctx.Done():
		t.Fatal("timed out waiting for approval decision")
		return agent.ApprovalDecision{}
	}
}
