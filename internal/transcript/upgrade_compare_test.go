package transcript

import "testing"

func TestCompareUpgradeOutputs_AcceptsBroadlyEquivalentSignals(t *testing.T) {
	previous := Output{
		Expect: Expect{
			Output:     TextExpectation{NumMessages: 2, Content: "hello"},
			Log:        TextExpectation{NumMessages: 1, Content: "warning"},
			Reasoning:  ReasoningExpectation{NumMessages: 3, Content: "think", NumStreams: 1, AllFlushed: true},
			FileChange: FileChangeExpectation{NumMessages: 1, Files: []string{"a.txt"}},
			Command:    CommandExpectation{NumMessages: 1, Executed: []string{"echo hello"}},
			Notice:     &NoticeExpectation{NumTurnFlushes: 1},
			Approvals:  []ApprovalExpectation{{}},
		},
	}
	current := Output{
		Expect: Expect{
			Output:     TextExpectation{NumMessages: 5, Content: "different"},
			Log:        TextExpectation{NumMessages: 2, Content: "different warning"},
			Reasoning:  ReasoningExpectation{NumMessages: 1, Content: "also different", NumStreams: 2, AllFlushed: true},
			FileChange: FileChangeExpectation{NumMessages: 2, Files: []string{"b.txt"}},
			Command:    CommandExpectation{NumMessages: 1, Executed: []string{"pwd"}},
			Notice:     &NoticeExpectation{NumTurnFlushes: 1},
			Approvals:  []ApprovalExpectation{{}},
		},
	}

	if err := CompareUpgradeOutputs(previous, current); err != nil {
		t.Fatalf("CompareUpgradeOutputs: %v", err)
	}
}

func TestCompareUpgradeOutputs_RejectsDroppedSignals(t *testing.T) {
	previous := Output{
		Expect: Expect{
			Output:     TextExpectation{NumMessages: 1, Content: "hello"},
			Log:        TextExpectation{NumMessages: 1, Content: "warning"},
			Reasoning:  ReasoningExpectation{NumMessages: 1, Content: "think", NumStreams: 1, AllFlushed: true},
			FileChange: FileChangeExpectation{NumMessages: 1, Files: []string{"a.txt"}},
			Command:    CommandExpectation{NumMessages: 1, Executed: []string{"echo hello"}},
			Notice:     &NoticeExpectation{NumTurnFlushes: 1},
			Approvals:  []ApprovalExpectation{{}},
		},
	}
	current := Output{}

	if err := CompareUpgradeOutputs(previous, current); err == nil {
		t.Fatal("CompareUpgradeOutputs succeeded, want error")
	}
}
