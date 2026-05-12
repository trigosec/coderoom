package participant_test

import (
	"errors"
	"testing"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
)

type fakeAgent struct{}

func (fakeAgent) Start() error               { return nil }
func (fakeAgent) Send(string) error          { return nil }
func (fakeAgent) Read() (agent.Event, error) { return agent.Event{}, errors.New("no events") }
func (fakeAgent) Interrupt() error           { return nil }
func (fakeAgent) Stop() error                { return nil }

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

	pStarting := newParticipant("starting")
	pStarting.Status = participant.StatusStarting

	pCrashed := newParticipant("crashed")
	pCrashed.Status = participant.StatusCrashed
	pCrashed.Agent = fakeAgent{}

	pIdle := newParticipant("idle")
	pIdle.Status = participant.StatusIdle
	pIdle.Agent = fakeAgent{}

	pWorking := newParticipant("working")
	pWorking.Status = participant.StatusWorking
	pWorking.Agent = fakeAgent{}

	_ = r.Add(pStarting)
	_ = r.Add(pCrashed)
	_ = r.Add(pIdle)
	_ = r.Add(pWorking)

	avail := r.ListAvailable()
	aliases := map[string]bool{}
	for _, p := range avail {
		aliases[p.Alias] = true
	}
	if aliases["starting"] {
		t.Fatal("expected starting participant to be excluded from ListAvailable")
	}
	if aliases["crashed"] {
		t.Fatal("expected crashed participant to be excluded from ListAvailable")
	}
	if !aliases["idle"] {
		t.Fatal("expected idle participant to be included in ListAvailable")
	}
	if !aliases["working"] {
		t.Fatal("expected working participant to be included in ListAvailable")
	}
}

func TestRegistry_StatusListsAndPredicates(t *testing.T) {
	r := participant.NewRegistry()

	pStarting := newParticipant("starting")
	pStarting.Status = participant.StatusStarting

	pCrashed := newParticipant("crashed")
	pCrashed.Status = participant.StatusCrashed

	pWorking := newParticipant("working")
	pWorking.Status = participant.StatusWorking

	_ = r.Add(pStarting)
	_ = r.Add(pCrashed)
	_ = r.Add(pWorking)

	if !r.HasStarting() {
		t.Fatal("expected HasStarting true")
	}
	if !r.HasCrashed() {
		t.Fatal("expected HasCrashed true")
	}
	if !r.HasWorking() {
		t.Fatal("expected HasWorking true")
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
