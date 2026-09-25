package interpreter

import (
	"github.com/trigosec/coderoom/internal/promptlang"
	"github.com/trigosec/coderoom/internal/session"
)

func (i *Interpreter) executeCancel(raw string, cancel promptlang.Cancel) {
	i.room.AppendUserInputRecord(raw, nil)
	i.publish(InputAccepted{Raw: raw})
	err := i.session.Execute(session.CancelCommand{Alias: cancel.Alias})
	i.drainSessionEvents(false)
	i.publish(StateChanged{Snapshot: i.captureSnapshot()})
	if err != nil {
		i.publish(SubmissionFailed{Raw: raw, Operation: "cancel", Code: ErrorExecutionFailed, Err: err})
		return
	}
	i.publish(SubmissionSucceeded{Raw: raw})
}
