package ui

import (
	"slices"
	"testing"

	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/session"
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
	if !hasRecord(m, recordKindSystem, "[ada joined]") {
		t.Errorf("expected [ada joined] system record; records: %v", m.records)
	}
}

func TestHandleEvent_agentStarting(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentStarting, Alias: "ada"})
	if !hasRecord(m, recordKindSystem, "[ada starting]") {
		t.Errorf("expected [ada starting] system record; records: %v", m.records)
	}
}

func TestHandleEvent_agentStopped(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentStarted, Alias: "ada"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if !hasRecord(m, recordKindSystem, "[ada left]") {
		t.Errorf("expected [ada left] system record; records: %v", m.records)
	}
}

func TestHandleEvent_agentCrashed(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentStarted, Alias: "ada"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentCrashed, Alias: "ada"})
	if !hasRecord(m, recordKindSystem, "[ada crashed]") {
		t.Errorf("expected [ada crashed] system record; records: %v", m.records)
	}
}

func TestHandleEvent_agentLog(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentLog, Alias: "ada", Text: "npm warn something"})
	if !hasRecord(m, recordKindLog, "npm warn something") {
		t.Errorf("expected log record with text; records: %v", m.records)
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
			if !hasRecord(m, recordKindSystem, tt.want) {
				t.Errorf("expected system record %q; records: %v", tt.want, m.records)
			}
		})
	}
}

// --- streaming ---

func TestHandleDelta_firstDeltaCreatesRecord(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	if len(m.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(m.records))
	}
	rec := m.records[m.streaming["ada"]]
	if rec.kind != recordKindAgentOutput {
		t.Errorf("expected agent output record, got kind %d", rec.kind)
	}
	if rec.alias != "ada" {
		t.Errorf("expected alias ada, got %q", rec.alias)
	}
	if rec.body != "hello" {
		t.Errorf("expected body 'hello', got %q", rec.body)
	}
}

func TestHandleDelta_subsequentDeltaAppendsInPlace(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: " world"})
	if len(m.records) != 1 {
		t.Fatalf("expected 1 record (in-place append), got %d", len(m.records))
	}
	if m.records[m.streaming["ada"]].body != "hello world" {
		t.Errorf("expected body 'hello world', got %q", m.records[m.streaming["ada"]].body)
	}
}

func TestHandleDelta_twoAgentsStreamConcurrently(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "a"})
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "bob", Text: "b"})
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "2"})
	if len(m.records) != 2 {
		t.Fatalf("expected 2 records, got %d: %v", len(m.records), m.records)
	}
	if m.records[m.streaming["ada"]].body != "a2" {
		t.Errorf("ada body wrong: %q", m.records[m.streaming["ada"]].body)
	}
	if m.records[m.streaming["bob"]].body != "b" {
		t.Errorf("bob body wrong: %q", m.records[m.streaming["bob"]].body)
	}
}

func TestHandleEvent_kindDoneClearsStreaming(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	m = pushEvent(m, session.Event{Kind: session.KindDone, Alias: "ada"})
	if _, ok := m.streaming["ada"]; ok {
		t.Error("expected streaming to be cleared after KindDone")
	}
}

func TestHandleEvent_agentStoppedClearsStreaming(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "mid-stream"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if _, ok := m.streaming["ada"]; ok {
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
	if !hasRecord(m, recordKindSystem, "no agents") {
		t.Errorf("expected no-agents hint in system records; records: %v", m.records)
	}
}

// --- showWho / showHelp ---

func TestShowWho_noAgents(t *testing.T) {
	m := makeReadyModel(t)
	m = m.showWho()
	if !hasRecord(m, recordKindSystem, "[no agents]") {
		t.Errorf("expected [no agents] system record; records: %v", m.records)
	}
}

func TestShowHelp_coversAllCommands(t *testing.T) {
	m := makeReadyModel(t)
	m = m.showHelp()
	for _, cmd := range []string{"/invite", "/stop", "/who", "/help", "@<alias>", "/quit"} {
		if !hasRecord(m, recordKindSystem, cmd) {
			t.Errorf("help output missing %q; records: %v", cmd, m.records)
		}
	}
}

// --- departed agent colour ---

func TestMarkDeparted_greyRepaintOnStop(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	m = pushEvent(m, session.Event{Kind: session.KindDone, Alias: "ada"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if !m.departed["ada"] {
		t.Error("expected ada in departed map after stop")
	}
	// colorFor must resolve ada to ColorDeparted so future renders (e.g. resize) use grey.
	if got := m.colorFor()("ada"); got != ColorDeparted {
		t.Errorf("colorFor(ada) after stop: want ColorDeparted, got %q", got)
	}
}

func TestMarkDeparted_greyRepaintOnCrash(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentCrashed, Alias: "ada"})
	if !m.departed["ada"] {
		t.Error("expected ada in departed map after crash")
	}
	// colorFor must resolve ada to ColorDeparted so future renders (e.g. resize) use grey.
	if got := m.colorFor()("ada"); got != ColorDeparted {
		t.Errorf("colorFor(ada) after crash: want ColorDeparted, got %q", got)
	}
}

func TestColorFor_departedReturnsGrey(t *testing.T) {
	m := makeReadyModel(t)
	m.departed["ada"] = true
	color := m.colorFor()("ada")
	if color != ColorDeparted {
		t.Errorf("expected ColorDeparted for departed agent, got %q", color)
	}
}

// --- streaming cleanup ---

func TestStreamingCleared_onStop(t *testing.T) {
	m := makeReadyModel(t)
	m.streaming["ada"] = 0
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if _, ok := m.streaming["ada"]; ok {
		t.Error("streaming should be cleared on agent stop")
	}
}
