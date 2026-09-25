package interpreter

import (
	"github.com/trigosec/coderoom/internal/promptlang"
	"github.com/trigosec/coderoom/internal/session"
)

func (i *Interpreter) executeInvite(raw string, invite promptlang.Invite) {
	i.room.AppendUserInputRecord(raw, nil)
	i.publish(InputAccepted{Raw: raw})
	err := i.session.Execute(session.InviteCommand{Alias: invite.Alias})
	i.drainSessionEvents(false)
	i.publish(StateChanged{Snapshot: i.captureSnapshot()})
	if err != nil {
		i.publish(SubmissionFailed{Raw: raw, Operation: "invite", Code: ErrorExecutionFailed, Err: err})
		return
	}
	i.publish(SubmissionSucceeded{Raw: raw})
}
