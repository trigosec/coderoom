package ui

import (
	"slices"
	"testing"

	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/session"
	"github.com/trigosec/coderoom/internal/ui/room/history"
)

// --- channelObserver ---

func TestChannelObserver_forwardsToQueue(t *testing.T) {
	q := newEventQueue()
	obs := channelObserver{queue: q}
	go obs.OnEvent(session.Event{Kind: session.KindAgentStarted, Alias: "ada"})
	got, ok := q.Pull()
	if !ok {
		t.Fatal("queue closed unexpectedly")
	}
	if got.Alias != "ada" {
		t.Errorf("expected alias ada, got %q", got.Alias)
	}
}

// --- handleEvent: records ---

func TestHandleEvent_agentStarted(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentStarted, Alias: "ada"})
	if !hasRecord(m, history.RecordKindSystem, "[ada joined]") {
		t.Errorf("expected [ada joined] system record; records: %v", m.history.Records())
	}
}

func TestHandleEvent_agentStarting(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentStarting, Alias: "ada"})
	if !hasRecord(m, history.RecordKindSystem, "[ada starting]") {
		t.Errorf("expected [ada starting] system record; records: %v", m.history.Records())
	}
}

func TestHandleEvent_agentStopped(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentStarted, Alias: "ada"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if !hasRecord(m, history.RecordKindSystem, "[ada left]") {
		t.Errorf("expected [ada left] system record; records: %v", m.history.Records())
	}
}

func TestHandleEvent_agentCrashed(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentStarted, Alias: "ada"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentCrashed, Alias: "ada"})
	if !hasRecord(m, history.RecordKindSystem, "[ada crashed]") {
		t.Errorf("expected [ada crashed] system record; records: %v", m.history.Records())
	}
}

func TestHandleEvent_agentLog(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentLog, Alias: "ada", Text: "npm warn something"})
	if !hasRecord(m, history.RecordKindLog, "npm warn something") {
		t.Errorf("expected log record with text; records: %v", m.history.Records())
	}
}

func TestHandleEvent_systemRecords(t *testing.T) {
	tests := []struct {
		name  string
		event session.Event
		want  string
	}{
		{"broadcast", session.Event{Kind: session.KindBroadcast, Text: "hello"}, "[all] hello"},
		{"sharedSend", session.Event{Kind: session.KindSharedSend, Alias: "ada", Text: "do it"}, "[→ ada] do it"},
		{"sharedNotice", session.Event{Kind: session.KindSharedNotice, Alias: "ada"}, "[notice → ada]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := makeReadyModel(t)
			m = pushEvent(m, tt.event)
			if !hasRecord(m, history.RecordKindSystem, tt.want) {
				t.Errorf("expected system record %q; records: %v", tt.want, m.history.Records())
			}
		})
	}
}

// --- streaming ---

func TestHandleDelta_firstDeltaCreatesRecord(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	recs := m.history.Records()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	idx, ok := m.history.StreamingIdx("ada")
	if !ok {
		t.Fatal("expected ada to be streaming")
	}
	rec := recs[idx]
	if rec.Kind != history.RecordKindAgentOutput {
		t.Errorf("expected agent output record, got kind %d", rec.Kind)
	}
	if rec.Alias != "ada" {
		t.Errorf("expected alias ada, got %q", rec.Alias)
	}
	if rec.Body != "hello" {
		t.Errorf("expected body 'hello', got %q", rec.Body)
	}
}

func TestHandleDelta_subsequentDeltaAppendsInPlace(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: " world"})
	recs := m.history.Records()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record (in-place append), got %d", len(recs))
	}
	idx, _ := m.history.StreamingIdx("ada")
	if recs[idx].Body != "hello world" {
		t.Errorf("expected body 'hello world', got %q", recs[idx].Body)
	}
}

func TestHandleDelta_twoAgentsStreamConcurrently(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "a"})
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "bob", Text: "b"})
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "2"})
	recs := m.history.Records()
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
	adaIdx, _ := m.history.StreamingIdx("ada")
	bobIdx, _ := m.history.StreamingIdx("bob")
	if recs[adaIdx].Body != "a2" {
		t.Errorf("ada body wrong: %q", recs[adaIdx].Body)
	}
	if recs[bobIdx].Body != "b" {
		t.Errorf("bob body wrong: %q", recs[bobIdx].Body)
	}
}

func TestHandleEvent_kindDoneClearsStreaming(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	m = pushEvent(m, session.Event{Kind: session.KindDone, Alias: "ada"})
	if m.history.IsStreaming("ada") {
		t.Error("expected streaming to be cleared after KindDone")
	}
}

func TestHandleEvent_agentStoppedClearsStreaming(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "mid-stream"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if m.history.IsStreaming("ada") {
		t.Error("streaming should be cleared when agent stops mid-turn")
	}
}

func TestRoutingFor(t *testing.T) {
	ps := []participant.Participant{{Alias: "ada"}, {Alias: "bob"}}
	if got := routingFor(Broadcast{Text: "hi"}, ps); !slices.Equal(got, []string{"ada", "bob"}) {
		t.Errorf("broadcast routing: got %v, want [ada bob]", got)
	}
	if got := routingFor(Send{Alias: "ada", Text: "hi"}, ps); !slices.Equal(got, []string{"ada"}) {
		t.Errorf("send routing: got %v, want [ada]", got)
	}
	if got := routingFor(Help{}, ps); got != nil {
		t.Errorf("help routing: got %v, want nil", got)
	}
}

// --- broadcastAll guard ---

func TestBroadcastAll_noAgentsShowsHint(t *testing.T) {
	m := makeReadyModel(t)
	m = m.broadcastAll("hello")
	if !hasRecord(m, history.RecordKindSystem, "no agents") {
		t.Errorf("expected no-agents hint in system records; records: %v", m.history.Records())
	}
}

// --- showWho / showHelp ---

func TestShowWho_noAgents(t *testing.T) {
	m := makeReadyModel(t)
	m = m.showWho()
	if !hasRecord(m, history.RecordKindSystem, "[no agents]") {
		t.Errorf("expected [no agents] system record; records: %v", m.history.Records())
	}
}

func TestShowHelp_coversAllCommands(t *testing.T) {
	m := makeReadyModel(t)
	m = m.showHelp()
	for _, cmd := range []string{"/invite", "/remove", "/cancel", "/who", "/help", "@<alias>", "/quit"} {
		if !hasRecord(m, history.RecordKindSystem, cmd) {
			t.Errorf("help output missing %q; records: %v", cmd, m.history.Records())
		}
	}
}

// --- departed agent colour ---

func TestMarkDeparted_greyRepaintOnStop(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	m = pushEvent(m, session.Event{Kind: session.KindDone, Alias: "ada"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if !m.history.IsDeparted("ada") {
		t.Error("expected ada in departed map after stop")
	}
}

func TestMarkDeparted_greyRepaintOnCrash(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentCrashed, Alias: "ada"})
	if !m.history.IsDeparted("ada") {
		t.Error("expected ada in departed map after crash")
	}
}

// --- streaming cleanup ---

func TestStreamingCleared_onStop(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "mid-stream"})
	if !m.history.IsStreaming("ada") {
		t.Fatal("expected ada to be streaming after delta")
	}
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if m.history.IsStreaming("ada") {
		t.Error("streaming should be cleared on agent stop")
	}
}
