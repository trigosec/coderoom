package staging

import (
	"strings"
	"testing"

	"github.com/trigosec/coderoom/internal/participant"
)

var renderStatusLineTests = []struct {
	name        string
	batch       *Batch
	blocked     []string
	wantSubs    []string
	wantNotSubs []string
}{
	{
		name:    "on_hold_none_busy",
		batch:   &Batch{},
		blocked: nil,
		wantSubs: []string{
			"Message on-hold.",
			"Participants busy: none.",
			"Press Esc to edit.",
		},
		wantNotSubs: []string{
			"Interrupt requested.",
			"Waiting to send…",
		},
	},
	{
		name: "interrupt_waiting",
		batch: &Batch{
			Interrupt: true,
		},
		blocked: []string{"ada"},
		wantSubs: []string{
			"Interrupt requested.",
			"Participants busy:",
			"ada",
			"Waiting to send…",
		},
		wantNotSubs: []string{
			"Press Esc to edit.",
		},
	},
}

func TestNewBatch_sortsAndCopiesBarrier(t *testing.T) {
	in := []string{"turing", "ada"}
	b := NewBatch("raw", Action{Kind: ActionBroadcast, Text: "hi"}, in)

	if got := strings.Join(b.Barrier, ","); got != "ada,turing" {
		t.Fatalf("expected sorted barrier, got %q", got)
	}

	in[0] = "mutated"
	if strings.Join(b.Barrier, ",") != "ada,turing" {
		t.Fatalf("expected barrier to be copied (immutable from input slice)")
	}
}

func TestBatchActiveTargets_respectsDiscarded(t *testing.T) {
	b := NewBatch("raw", Action{Kind: ActionBroadcast, Text: "hi"}, []string{"ada", "turing"})
	b.MarkDiscarded("turing")
	got := strings.Join(b.ActiveTargets(), ",")
	if got != "ada" {
		t.Fatalf("expected only ada active, got %q", got)
	}
}

func TestBatchBlockedTargets_discardsMissingAndSorts(t *testing.T) {
	b := NewBatch("raw", Action{Kind: ActionBroadcast, Text: "hi"}, []string{"turing", "missing", "ada"})

	statusByAlias := func(alias string) (participant.Status, bool) {
		switch alias {
		case "ada":
			return participant.StatusWorking, true
		case "turing":
			return participant.StatusIdle, true
		default:
			return "", false
		}
	}

	blocked := b.BlockedTargets(statusByAlias)
	if got := strings.Join(blocked, ","); got != "ada" {
		t.Fatalf("expected only ada blocked, got %q", got)
	}
	if !b.Discarded["missing"] {
		t.Fatalf("expected missing alias to be discarded")
	}
	if got := strings.Join(b.ActiveTargets(), ","); got != "ada,turing" {
		t.Fatalf("expected missing alias to be removed from active targets, got %q", got)
	}
}

func TestRenderStatusLine_includesModeAndBusySummary(t *testing.T) {
	for _, tt := range renderStatusLineTests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderStatusLine(tt.batch, tt.blocked, func(string) string { return "" })
			for _, sub := range tt.wantSubs {
				if !strings.Contains(got, sub) {
					t.Fatalf("expected status line to contain %q; got %q", sub, got)
				}
			}
			for _, sub := range tt.wantNotSubs {
				if strings.Contains(got, sub) {
					t.Fatalf("expected status line to not contain %q; got %q", sub, got)
				}
			}
		})
	}
}
