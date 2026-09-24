package interpreter

import "testing"

func TestSubmitContract_helpPublishesCommandMetadata(t *testing.T) {
	interp, sess, events := newSubmitContractInterpreter(t)

	mustSubmit(t, interp.Submit("/help"))
	receiveSubmitEvent[InputAccepted](t, events)
	receiveSubmitEvent[StateChanged](t, events)
	help := receiveSubmitEvent[HelpListed](t, events)
	tests := []struct {
		name    string
		entries []HelpEntry
		want    []string
	}{
		{name: "commands", entries: help.Commands, want: expectedHelpCommandUsages()},
		{name: "messages", entries: help.Messages, want: []string{"@<alias> <text>", "<text>"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.entries) != len(tt.want) {
				t.Fatalf("entry count = %d, want %d: %#v", len(tt.entries), len(tt.want), tt.entries)
			}
			for _, usage := range tt.want {
				if !containsHelpUsage(tt.entries, usage) {
					t.Errorf("missing usage %q: %#v", usage, tt.entries)
				}
			}
		})
	}
	receiveSubmitEvent[SubmissionSucceeded](t, events)
	assertNoSubmitExecution(t, sess.executed)
	assertNoSubmitEvent(t, events)
}

func expectedHelpCommandUsages() []string {
	return []string{
		"/policy enable send-notices",
		"/policy enable echo-invites",
		"/invite <alias>",
		"/remove <alias>",
		"/cancel <alias>",
		"/handoff <from> <to>",
		"/shell <program>",
		"/def <name> /shell <program>",
		"/<name>",
		"/loop @<alias> <prompt> /until /<name> /max <turns>",
		"/who",
		"/help",
		"/quit",
	}
}

func containsHelpUsage(entries []HelpEntry, usage string) bool {
	for _, entry := range entries {
		if entry.Usage == usage {
			return true
		}
	}
	return false
}
