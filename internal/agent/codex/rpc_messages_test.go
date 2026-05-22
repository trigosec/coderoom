package codex

import (
	"encoding/json"
	"testing"

	"github.com/trigosec/coderoom/internal/agent"
)

// TestMessageFromEnvelope_flushContentIsZero encodes the invariant: every
// ModeFlush message produced by the adapter must carry zero-value content.
// Add a row here whenever a new wire event produces a ModeFlush.
func TestMessageFromEnvelope_flushContentIsZero(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		wantFlush agent.MessageContent // zero-value of the expected flush content type
	}{
		{
			"turn/completed",
			`{"method":"turn/completed","params":{"turn":{"id":"t1"}}}`,
			agent.Output{},
		},
		{
			"item/reasoning/summaryPartAdded",
			`{"method":"item/reasoning/summaryPartAdded","params":{"itemId":"i1","threadId":"th1","turnId":"t1"}}`,
			agent.Reasoning{},
		},
		{
			// item/completed emits ModeStream{ExitCode} then ModeFlush{}; only the flush is checked.
			"item/completed commandExecution",
			`{"method":"item/completed","params":{"turnId":"t1","threadId":"th1","completedAtMs":0,"item":{"type":"commandExecution","id":"cmd1","command":"ls","cwd":"/","status":"completed","commandActions":[]}}}`,
			agent.Command{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var env rpcEnvelope
			if err := json.Unmarshal([]byte(tc.line), &env); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			msgs, err := messageFromEnvelope(env)
			if err != nil {
				t.Fatalf("messageFromEnvelope: %v", err)
			}
			var sawFlush bool
			for _, msg := range msgs {
				if msg.Mode != agent.ModeFlush {
					continue
				}
				sawFlush = true
				if msg.Content != tc.wantFlush {
					t.Errorf("ModeFlush content = %T(%+v), want zero-value %T", msg.Content, msg.Content, tc.wantFlush)
				}
			}
			if !sawFlush {
				t.Errorf("no ModeFlush produced; expected flush of type %T", tc.wantFlush)
			}
		})
	}
}
