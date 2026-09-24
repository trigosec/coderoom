package interpreter

import (
	"github.com/trigosec/coderoom/internal/promptlang"
	"github.com/trigosec/coderoom/internal/session"
)

func (i *Interpreter) executeRemove(raw string, remove promptlang.Remove) {
	i.room.AppendUserInputRecord(raw, nil)
	i.publish(InputAccepted{Raw: raw})
	err := i.session.Execute(session.RemoveCommand{Alias: remove.Alias})
	i.drainSessionEvents(false)
	i.publish(StateChanged{Snapshot: i.captureSnapshot()})
	if err != nil {
		i.publish(SubmissionFailed{Raw: raw, Operation: "remove", Err: err})
		return
	}
	i.publish(SubmissionSucceeded{Raw: raw})
}
