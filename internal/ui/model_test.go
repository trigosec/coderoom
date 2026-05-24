package ui

import (
	"slices"
	"testing"

	"github.com/trigosec/coderoom/internal/agent"
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
		t.Errorf("expected [ada joined] system record; records: %v", m.room.HistoryRecords())
	}
}

func TestHandleEvent_agentStarting(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentStarting, Alias: "ada"})
	if !hasRecord(m, history.RecordKindSystem, "[ada starting]") {
		t.Errorf("expected [ada starting] system record; records: %v", m.room.HistoryRecords())
	}
}

func TestHandleEvent_agentStopped(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentStarted, Alias: "ada"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if !hasRecord(m, history.RecordKindSystem, "[ada left]") {
		t.Errorf("expected [ada left] system record; records: %v", m.room.HistoryRecords())
	}
}

func TestHandleEvent_agentCrashed(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentStarted, Alias: "ada"})
	m = pushEvent(m, session.Event{Kind: session.KindAgentCrashed, Alias: "ada"})
	if !hasRecord(m, history.RecordKindSystem, "[ada crashed]") {
		t.Errorf("expected [ada crashed] system record; records: %v", m.room.HistoryRecords())
	}
}

func TestHandleEvent_agentLog(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentLog, Alias: "ada", Text: "npm warn something"})
	if !hasRecord(m, history.RecordKindLog, "npm warn something") {
		t.Errorf("expected log record with text; records: %v", m.room.HistoryRecords())
	}
}

func TestHandleEvent_systemRecords(t *testing.T) {
	tests := []struct {
		name  string
		event session.Event
		want  string
	}{
		{"sharedNotice", session.Event{Kind: session.KindSharedNotice, Alias: "ada"}, "[notice → ada]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := makeReadyModel(t)
			m = pushEvent(m, tt.event)
			if !hasRecord(m, history.RecordKindSystem, tt.want) {
				t.Errorf("expected system record %q; records: %v", tt.want, m.room.HistoryRecords())
			}
		})
	}
}

func TestHandleEvent_broadcastAndSharedSendProduceNoSystemRecord(t *testing.T) {
	events := []session.Event{
		{Kind: session.KindBroadcast, Text: "hello"},
		{Kind: session.KindSharedSend, Alias: "ada", Text: "do it"},
	}
	for _, e := range events {
		m := makeReadyModel(t)
		m = pushEvent(m, e)
		if len(m.room.HistoryRecords()) != 0 {
			t.Errorf("expected no system record for %v; got %v", e.Kind, m.room.HistoryRecords())
		}
	}
}

// --- streaming ---

func TestHandleDelta_firstDeltaCreatesRecord(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "out1", Mode: agent.ModeStream, Content: agent.Output{Text: "hello"},
	}})
	recs := m.room.HistoryRecords()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	idx, ok := m.room.StreamingIdx("ada")
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
	if rec.Msg == nil {
		t.Fatal("expected record to carry Msg, got nil")
	}
	out, ok := rec.Msg.Content.(agent.Output)
	if !ok {
		t.Fatalf("expected Output content, got %T", rec.Msg.Content)
	}
	if out.Text != "hello" {
		t.Errorf("expected body 'hello', got %q", out.Text)
	}
}

func TestHandleDelta_subsequentDeltaAppendsInPlace(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "out1", Mode: agent.ModeStream, Content: agent.Output{Text: "hello"},
	}})
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "out1", Mode: agent.ModeStream, Content: agent.Output{Text: " world"},
	}})
	recs := m.room.HistoryRecords()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record (in-place append), got %d", len(recs))
	}
	idx, _ := m.room.StreamingIdx("ada")
	if recs[idx].Msg == nil {
		t.Fatal("expected record to carry Msg, got nil")
	}
	out, ok := recs[idx].Msg.Content.(agent.Output)
	if !ok {
		t.Fatalf("expected Output content, got %T", recs[idx].Msg.Content)
	}
	if out.Text != "hello world" {
		t.Errorf("expected body 'hello world', got %q", out.Text)
	}
}

func TestHandleDelta_twoAgentsStreamConcurrently(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "out-ada", Mode: agent.ModeStream, Content: agent.Output{Text: "a"},
	}})
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "bob", Msg: &agent.Message{
		StreamID: "out-bob", Mode: agent.ModeStream, Content: agent.Output{Text: "b"},
	}})
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "out-ada", Mode: agent.ModeStream, Content: agent.Output{Text: "2"},
	}})
	recs := m.room.HistoryRecords()
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
	adaIdx, _ := m.room.StreamingIdx("ada")
	bobIdx, _ := m.room.StreamingIdx("bob")
	if recs[adaIdx].Msg == nil {
		t.Fatal("expected ada record to carry Msg, got nil")
	}
	adaOut, ok := recs[adaIdx].Msg.Content.(agent.Output)
	if !ok {
		t.Fatalf("expected ada Output content, got %T", recs[adaIdx].Msg.Content)
	}
	if adaOut.Text != "a2" {
		t.Errorf("ada body wrong: %q", adaOut.Text)
	}
	if recs[bobIdx].Msg == nil {
		t.Fatal("expected bob record to carry Msg, got nil")
	}
	bobOut, ok := recs[bobIdx].Msg.Content.(agent.Output)
	if !ok {
		t.Fatalf("expected bob Output content, got %T", recs[bobIdx].Msg.Content)
	}
	if bobOut.Text != "b" {
		t.Errorf("bob body wrong: %q", bobOut.Text)
	}
}

func TestHandleEvent_kindDoneClearsStreaming(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "out1", Mode: agent.ModeStream, Content: agent.Output{Text: "hello"},
	}})
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "turn1", Mode: agent.ModeFlush, Content: agent.Output{},
	}})
	if m.room.IsStreaming("ada") {
		t.Error("expected streaming to be cleared after turn flush")
	}
}

func TestHandleEvent_agentStoppedClearsStreaming(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "out1", Mode: agent.ModeStream, Content: agent.Output{Text: "mid-stream"},
	}})
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if m.room.IsStreaming("ada") {
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
		t.Errorf("expected no-agents hint in system records; records: %v", m.room.HistoryRecords())
	}
}

// --- showWho / showHelp ---

func TestShowWho_noAgents(t *testing.T) {
	m := makeReadyModel(t)
	m = m.showWho()
	if !hasRecord(m, history.RecordKindSystem, "[no agents]") {
		t.Errorf("expected [no agents] system record; records: %v", m.room.HistoryRecords())
	}
}

func TestShowHelp_coversAllCommands(t *testing.T) {
	m := makeReadyModel(t)
	m = m.showHelp()
	for _, cmd := range []string{"/invite", "/remove", "/cancel", "/who", "/help", "@<alias>", "/quit"} {
		if !hasRecord(m, history.RecordKindSystem, cmd) {
			t.Errorf("help output missing %q; records: %v", cmd, m.room.HistoryRecords())
		}
	}
}

// --- departed agent colour ---

func TestMarkDeparted_greyRepaintOnStop(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "out1", Mode: agent.ModeStream, Content: agent.Output{Text: "hello"},
	}})
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "turn1", Mode: agent.ModeFlush, Content: agent.Output{},
	}})
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if !m.room.IsDeparted("ada") {
		t.Error("expected ada in departed map after stop")
	}
}

func TestMarkDeparted_greyRepaintOnCrash(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "out1", Mode: agent.ModeStream, Content: agent.Output{Text: "hello"},
	}})
	m = pushEvent(m, session.Event{Kind: session.KindAgentCrashed, Alias: "ada"})
	if !m.room.IsDeparted("ada") {
		t.Error("expected ada in departed map after crash")
	}
}

// --- reasoning streaming ---

func TestHandleReasoningDelta_firstDeltaCreatesRecord(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "reason1", Mode: agent.ModeStream, Content: agent.Reasoning{Text: "let me think"},
	}})
	recs := m.room.HistoryRecords()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].Kind != history.RecordKindReasoning {
		t.Errorf("expected reasoning record, got kind %d", recs[0].Kind)
	}
	if recs[0].Msg == nil {
		t.Fatal("expected record to carry Msg, got nil")
	}
	thought, ok := recs[0].Msg.Content.(agent.Reasoning)
	if !ok {
		t.Fatalf("expected Reasoning content, got %T", recs[0].Msg.Content)
	}
	if thought.Text != "let me think" {
		t.Errorf("expected body 'let me think', got %q", thought.Text)
	}
	if !m.room.IsReasoningStreaming("ada") {
		t.Error("expected ada to be reasoning-streaming")
	}
}

func TestHandleReasoningDelta_appendsInPlace(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "reason1", Mode: agent.ModeStream, Content: agent.Reasoning{Text: "step 1"},
	}})
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "reason1", Mode: agent.ModeStream, Content: agent.Reasoning{Text: " step 2"},
	}})
	recs := m.room.HistoryRecords()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record (in-place append), got %d", len(recs))
	}
	if recs[0].Msg == nil {
		t.Fatal("expected record to carry Msg, got nil")
	}
	thought, ok := recs[0].Msg.Content.(agent.Reasoning)
	if !ok {
		t.Fatalf("expected Reasoning content, got %T", recs[0].Msg.Content)
	}
	if thought.Text != "step 1 step 2" {
		t.Errorf("expected body 'step 1 step 2', got %q", thought.Text)
	}
}

func TestHandleReasoningDelta_independentOfOutputStreaming(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "reason1", Mode: agent.ModeStream, Content: agent.Reasoning{Text: "thinking"},
	}})
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "out1", Mode: agent.ModeStream, Content: agent.Output{Text: "responding"},
	}})
	recs := m.room.HistoryRecords()
	if len(recs) != 2 {
		t.Fatalf("expected 2 records (reasoning + output), got %d", len(recs))
	}
	if recs[0].Kind != history.RecordKindReasoning {
		t.Errorf("expected first record to be reasoning, got kind %d", recs[0].Kind)
	}
	if recs[1].Kind != history.RecordKindAgentOutput {
		t.Errorf("expected second record to be agent output, got kind %d", recs[1].Kind)
	}
}

func TestKindDone_clearsReasoningStreaming(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "reason1", Mode: agent.ModeStream, Content: agent.Reasoning{Text: "thinking"},
	}})
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "turn1", Mode: agent.ModeFlush, Content: agent.Output{},
	}})
	if m.room.IsReasoningStreaming("ada") {
		t.Error("expected reasoning streaming to be cleared after turn flush")
	}
}

// --- streaming cleanup ---

func TestStreamingCleared_onStop(t *testing.T) {
	m := makeReadyModel(t)
	m = pushEvent(m, session.Event{Kind: session.KindAgentMessage, Alias: "ada", Msg: &agent.Message{
		StreamID: "out1", Mode: agent.ModeStream, Content: agent.Output{Text: "mid-stream"},
	}})
	if !m.room.IsStreaming("ada") {
		t.Fatal("expected ada to be streaming after delta")
	}
	m = pushEvent(m, session.Event{Kind: session.KindAgentStopped, Alias: "ada"})
	if m.room.IsStreaming("ada") {
		t.Error("streaming should be cleared on agent stop")
	}
}
