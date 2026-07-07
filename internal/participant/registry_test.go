package participant_test

import (
	"errors"
	"testing"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
)

type fakeAgent struct{}

func (fakeAgent) Start() error { return nil }
func (fakeAgent) Send(string) (agent.StreamID, error) {
	return "", nil
}
func (fakeAgent) SendNotice(string) (agent.StreamID, error) { return "", nil }
func (fakeAgent) Read() (agent.Message, error)              { return agent.Message{}, errors.New("no messages") }
func (fakeAgent) Interrupt() error                          { return nil }
func (fakeAgent) Stop() error                               { return nil }

func newParticipant(alias string) *participant.Participant {
	return &participant.Participant{
		Alias:      alias,
		Role:       participant.RoleBuilder,
		Initiative: participant.InitiativeManual,
		Status:     participant.StatusIdle,
	}
}

func TestRegistry_AddAndGet(t *testing.T) {
	r := participant.NewRegistry()
	p := newParticipant("ada")

	if err := r.Add(p); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, ok := r.Get("ada")
	if !ok {
		t.Fatal("Get: expected participant, got nothing")
	}
	if got != p {
		t.Errorf("Get returned wrong participant")
	}
}

func TestRegistry_Add_nil(t *testing.T) {
	r := participant.NewRegistry()
	if err := r.Add(nil); err == nil {
		t.Fatal("expected error on nil participant, got nil")
	}
}

func TestRegistry_Add_emptyAlias(t *testing.T) {
	r := participant.NewRegistry()
	if err := r.Add(&participant.Participant{}); err == nil {
		t.Fatal("expected error on empty alias, got nil")
	}
}

func TestRegistry_Add_duplicateAlias(t *testing.T) {
	r := participant.NewRegistry()
	if err := r.Add(newParticipant("ada")); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := r.Add(newParticipant("ada")); err == nil {
		t.Fatal("expected error on duplicate alias, got nil")
	}
}

func TestRegistry_Get_missing(t *testing.T) {
	r := participant.NewRegistry()
	_, ok := r.Get("nobody")
	if ok {
		t.Fatal("expected ok=false for unknown alias")
	}
}

func TestRegistry_Remove(t *testing.T) {
	r := participant.NewRegistry()
	if err := r.Add(newParticipant("ada")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := r.Remove("ada"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := r.Get("ada"); ok {
		t.Error("participant still present after Remove")
	}
}

func TestRegistry_Remove_missing(t *testing.T) {
	r := participant.NewRegistry()
	if err := r.Remove("nobody"); err == nil {
		t.Fatal("expected error removing unknown alias, got nil")
	}
}

func TestRegistry_List(t *testing.T) {
	r := participant.NewRegistry()
	_ = r.Add(newParticipant("ada"))
	_ = r.Add(newParticipant("turing"))

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 participants, got %d", len(list))
	}
	aliases := map[string]bool{}
	for _, p := range list {
		aliases[p.Alias] = true
	}
	for _, want := range []string{"ada", "turing"} {
		if !aliases[want] {
			t.Errorf("expected alias %q in list", want)
		}
	}
}

func TestRegistry_List_empty(t *testing.T) {
	r := participant.NewRegistry()
	if list := r.List(); len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestRegistry_ListAvailable_filtersByAgentAndStatus(t *testing.T) {
	r := participant.NewRegistry()
	addAvailableTestParticipant(t, r, "starting", participant.StatusStarting, false)
	addAvailableTestParticipant(t, r, "crashed", participant.StatusCrashed, true)
	addAvailableTestParticipant(t, r, "preparing", participant.StatusPreparing, true)
	addAvailableTestParticipant(t, r, "keepalive", participant.StatusKeepalive, true)
	addAvailableTestParticipant(t, r, "idle", participant.StatusIdle, true)
	addAvailableTestParticipant(t, r, "working", participant.StatusWorking, true)

	avail := r.ListAvailable()
	aliases := map[string]bool{}
	for _, p := range avail {
		aliases[p.Alias] = true
	}
	assertAvailableAlias(t, aliases, "starting", false)
	assertAvailableAlias(t, aliases, "crashed", false)
	assertAvailableAlias(t, aliases, "preparing", false)
	assertAvailableAlias(t, aliases, "keepalive", false)
	assertAvailableAlias(t, aliases, "idle", true)
	assertAvailableAlias(t, aliases, "working", true)
}

func addAvailableTestParticipant(t *testing.T, r *participant.Registry, alias string, status participant.Status, attachAgent bool) {
	t.Helper()
	p := newParticipant(alias)
	p.Status = status
	if attachAgent {
		p.Agent = fakeAgent{}
	}
	if err := r.Add(p); err != nil {
		t.Fatalf("Add(%q): %v", alias, err)
	}
}

func assertAvailableAlias(t *testing.T, aliases map[string]bool, alias string, want bool) {
	t.Helper()
	if aliases[alias] == want {
		return
	}
	if want {
		t.Fatalf("expected %q to be included in ListAvailable", alias)
	}
	t.Fatalf("expected %q to be excluded from ListAvailable", alias)
}

func TestRegistry_StatusListsAndPredicates(t *testing.T) {
	r := participant.NewRegistry()

	pStarting := newParticipant("starting")
	pStarting.Status = participant.StatusStarting

	pCrashed := newParticipant("crashed")
	pCrashed.Status = participant.StatusCrashed

	pWorking := newParticipant("working")
	pWorking.Status = participant.StatusWorking

	pKeepalive := newParticipant("keepalive")
	pKeepalive.Status = participant.StatusKeepalive

	_ = r.Add(pStarting)
	_ = r.Add(pCrashed)
	_ = r.Add(pWorking)
	_ = r.Add(pKeepalive)

	if !r.HasStarting() {
		t.Fatal("expected HasStarting true")
	}
	if !r.HasCrashed() {
		t.Fatal("expected HasCrashed true")
	}
	if !r.HasWorking() {
		t.Fatal("expected HasWorking true")
	}
	if !r.HasKeepalive() {
		t.Fatal("expected HasKeepalive true")
	}

	if len(r.ListStarting()) != 1 {
		t.Fatalf("expected 1 starting, got %d", len(r.ListStarting()))
	}
	if len(r.ListCrashed()) != 1 {
		t.Fatalf("expected 1 crashed, got %d", len(r.ListCrashed()))
	}
	if len(r.ListWorking()) != 1 {
		t.Fatalf("expected 1 working, got %d", len(r.ListWorking()))
	}
}

func TestParticipantSnapshot_copiesOpenStreams(t *testing.T) {
	p := newParticipant("ada")
	p.Status = participant.StatusWorking
	if err := p.TrackStream(agent.StreamID("out1")); err != nil {
		t.Fatalf("TrackStream: %v", err)
	}

	snap := p.Snapshot()
	if _, ok := snap.OpenStreams[agent.StreamID("out1")]; !ok {
		t.Fatal("expected snapshot to include tracked stream")
	}

	snap.OpenStreams[agent.StreamID("out2")] = struct{}{}
	if _, ok := p.OpenStreams[agent.StreamID("out2")]; ok {
		t.Fatal("expected snapshot stream mutation to not affect participant state")
	}
}

func TestParticipantMarkIdle_rejectsOpenStreams(t *testing.T) {
	p := newParticipant("ada")
	const anchor = agent.StreamID("anchor1")
	if err := p.PrepareForWork(testNow()); err != nil {
		t.Fatalf("PrepareForWork: %v", err)
	}
	if err := p.BeginWorking(testNow(), anchor); err != nil {
		t.Fatalf("BeginWorking: %v", err)
	}
	// Anchor is still open in OpenStreams — BecomeIdle must reject.
	if err := p.BecomeIdle(testNow()); err == nil {
		t.Fatal("expected BecomeIdle to reject open streams")
	}
}

func TestParticipantCloseStream_onlyAnchorTriggersIdle(t *testing.T) {
	p := newParticipant("ada")
	if err := p.PrepareForWork(testNow()); err != nil {
		t.Fatalf("PrepareForWork: %v", err)
	}
	if err := p.BeginWorking(testNow(), agent.StreamID("anchor")); err != nil {
		t.Fatalf("BeginWorking: %v", err)
	}
	if err := p.TrackStream(agent.StreamID("out1")); err != nil {
		t.Fatalf("TrackStream out1: %v", err)
	}

	shouldIdle, err := p.CloseStream(agent.StreamID("out1"))
	if err != nil {
		t.Fatalf("CloseStream out1: %v", err)
	}
	if shouldIdle {
		t.Fatal("expected non-anchor close to keep participant working")
	}

	shouldIdle, err = p.CloseStream(agent.StreamID("anchor"))
	if err != nil {
		t.Fatalf("CloseStream anchor: %v", err)
	}
	if !shouldIdle {
		t.Fatal("expected anchor close to trigger idle")
	}
}

func testNow() time.Time { return time.Unix(123, 0) }
