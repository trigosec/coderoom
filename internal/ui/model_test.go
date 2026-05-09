package ui

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/session"
)

// makeReadyModel returns a Model that has processed one WindowSizeMsg so the
// viewport is initialised and syncViewport calls are live.
func makeReadyModel(t *testing.T) Model {
	t.Helper()
	m := New(".")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(Model)
}

func makeReadyModelWithHeight(t *testing.T, height int) Model {
	t.Helper()
	m := New(".")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: height})
	return next.(Model)
}

// pushEvent sends a session event into the model via Update and returns the result.
func pushEvent(m Model, e session.Event) Model {
	next, _ := m.Update(sessionEventMsg(e))
	return next.(Model)
}

// hasRecord reports whether any record of the given kind contains text in its body.
func hasRecord(m Model, kind recordKind, text string) bool {
	for _, r := range m.records {
		if r.kind == kind && strings.Contains(r.body, text) {
			return true
		}
	}
	return false
}

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

// --- handleEnter: echo and routing ---

func TestHandleEnter_echoesUserInput(t *testing.T) {
	m := makeReadyModel(t)
	m.input.SetValue("/who")
	m, _ = m.handleEnter()
	if len(m.records) == 0 {
		t.Fatal("expected at least one record after enter")
	}
	if m.records[0].kind != recordKindUserInput {
		t.Errorf("expected first record to be user input, got kind %d", m.records[0].kind)
	}
	if m.records[0].body != "/who" {
		t.Errorf("expected body '/who', got %q", m.records[0].body)
	}
}

func TestAltEnter_insertsNewlineWithoutSubmitting(t *testing.T) {
	m := makeReadyModel(t)
	m.input.SetValue("hello")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	m2 := next.(Model)

	if got := m2.input.Value(); got != "hello\n" {
		t.Fatalf("expected Alt+Enter to insert newline, got %q", got)
	}
	if len(m2.records) != 0 {
		t.Fatalf("expected no records (no submit) after Alt+Enter, got %d", len(m2.records))
	}
}

func TestEnter_submitsAndClearsInput(t *testing.T) {
	m := makeReadyModel(t)
	m.input.SetValue("hello")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := next.(Model)

	if got := m2.input.Value(); got != "" {
		t.Fatalf("expected input cleared after submit, got %q", got)
	}
	if len(m2.records) == 0 || m2.records[0].kind != recordKindUserInput || m2.records[0].body != "hello" {
		t.Fatalf("expected first record to echo submitted input; records: %v", m2.records)
	}
}

func TestEnter_whitespaceOnlyDoesNotCreateRecord(t *testing.T) {
	m := makeReadyModel(t)
	m.input.SetValue("   \n\t ")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := next.(Model)

	if got := m2.input.Value(); got != "" {
		t.Fatalf("expected input cleared even for whitespace-only submit, got %q", got)
	}
	if len(m2.records) != 0 {
		t.Fatalf("expected no records for whitespace-only submit, got %d", len(m2.records))
	}
}

func TestPgDn_scrollsViewportAndDoesNotAffectInput(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)
	m.input.SetValue("draft")

	for i := range 40 {
		m = m.appendRecord(record{kind: recordKindSystem, body: "line " + strconv.Itoa(i)})
	}
	m.viewport.GotoTop()
	start := m.viewport.YOffset

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m2 := next.(Model)

	if m2.viewport.YOffset <= start {
		t.Fatalf("expected PgDn to scroll viewport down; before=%d after=%d", start, m2.viewport.YOffset)
	}
	if got := m2.input.Value(); got != "draft" {
		t.Fatalf("expected PgDn not to change input, got %q", got)
	}
}

func TestPgUp_scrollsViewportUpAndDoesNotAffectInput(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)
	m.input.SetValue("draft")

	for i := range 40 {
		m = m.appendRecord(record{kind: recordKindSystem, body: "line " + strconv.Itoa(i)})
	}
	m.viewport.GotoBottom()
	start := m.viewport.YOffset

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m2 := next.(Model)

	if m2.viewport.YOffset >= start {
		t.Fatalf("expected PgUp to scroll viewport up; before=%d after=%d", start, m2.viewport.YOffset)
	}
	if got := m2.input.Value(); got != "draft" {
		t.Fatalf("expected PgUp not to change input, got %q", got)
	}
}

func TestDelta_whenAtBottom_keepsViewportAtBottom(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)
	for i := range 40 {
		m = m.appendRecord(record{kind: recordKindSystem, body: "line " + strconv.Itoa(i)})
	}
	m.viewport.GotoBottom()
	if !m.viewport.AtBottom() {
		t.Fatal("expected to start at bottom")
	}

	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	if !m.viewport.AtBottom() {
		t.Fatal("expected viewport to remain at bottom after delta when already at bottom")
	}
}

func TestDelta_whenScrolledUp_doesNotForceViewportToBottom(t *testing.T) {
	m := makeReadyModelWithHeight(t, 10)
	for i := range 40 {
		m = m.appendRecord(record{kind: recordKindSystem, body: "line " + strconv.Itoa(i)})
	}
	m.viewport.GotoTop()
	if m.viewport.AtBottom() {
		t.Fatal("expected not to be at bottom when positioned at top")
	}
	start := m.viewport.YOffset

	m = pushEvent(m, session.Event{Kind: session.KindDelta, Alias: "ada", Text: "hello"})
	if m.viewport.AtBottom() {
		t.Fatal("expected viewport not to jump to bottom when user is scrolled up")
	}
	if m.viewport.YOffset != start {
		t.Fatalf("expected viewport y-offset unchanged when scrolled up; before=%d after=%d", start, m.viewport.YOffset)
	}
}

func TestInputHeight_isCappedAndDoesNotCollapseViewport(t *testing.T) {
	m := makeReadyModelWithHeight(t, 30) // max input height = min(8, 30/3=10) => 8

	m.input.SetValue(strings.Repeat("x\n", 20) + "x") // 21 lines
	m = m.resizeForInput()

	if got := m.input.Height(); got != 8 {
		t.Fatalf("expected input height capped at 8, got %d", got)
	}
	if m.viewport.Height <= 0 {
		t.Fatalf("expected viewport height to stay positive, got %d", m.viewport.Height)
	}
}

func TestResizeForInput_preservesBottomAnchor(t *testing.T) {
	m := makeReadyModelWithHeight(t, 12)
	for i := range 50 {
		m = m.appendRecord(record{kind: recordKindSystem, body: "line " + strconv.Itoa(i)})
	}
	m.viewport.GotoBottom()
	if !m.viewport.AtBottom() {
		t.Fatal("expected to start at bottom")
	}

	m.input.SetValue("a\nb\nc")
	m = m.resizeForInput()
	if !m.viewport.AtBottom() {
		t.Fatal("expected resizeForInput to keep viewport anchored to bottom")
	}
}

func TestCtrlG_withoutEditorAddsSystemRecordAndPreservesBuffer(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	m := makeReadyModel(t)
	m.input.SetValue("draft")

	m2, _ := m.startEditorCompose()
	if got := m2.input.Value(); got != "draft" {
		t.Fatalf("expected input preserved when editor is unset, got %q", got)
	}
	if !hasRecord(m2, recordKindSystem, "no editor configured") {
		t.Fatalf("expected system record about editor configuration; records: %v", m2.records)
	}
}

func TestEditorCompose_cancelRestoresPriorBuffer(t *testing.T) {
	m := makeReadyModel(t)
	m.input.SetValue("before")

	next, _ := m.Update(editorComposeMsg{
		prior:    "before",
		canceled: true,
		err:      os.ErrInvalid,
	})
	m2 := next.(Model)
	if got := m2.input.Value(); got != "before" {
		t.Fatalf("expected cancel to restore prior buffer, got %q", got)
	}
}

func TestEditorCompose_successReplacesBuffer(t *testing.T) {
	m := makeReadyModel(t)
	m.input.SetValue("before")

	next, _ := m.Update(editorComposeMsg{
		prior:   "before",
		content: "after",
	})
	m2 := next.(Model)
	if got := m2.input.Value(); got != "after" {
		t.Fatalf("expected success to replace buffer, got %q", got)
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
	m, _ = m.broadcastAll("hello")
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
