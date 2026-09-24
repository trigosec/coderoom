package interpreter

import (
	"github.com/trigosec/coderoom/internal/promptlang"
	"github.com/trigosec/coderoom/internal/session"
)

func (i *Interpreter) executePolicyEnable(raw string, enable promptlang.PolicyEnable) {
	i.room.AppendUserInputRecord(raw, nil)
	i.publish(InputAccepted{Raw: raw})
	err := i.session.Execute(session.EnablePolicyCommand{Name: enable.Name})
	i.drainSessionEvents(false)
	i.publish(StateChanged{Snapshot: i.captureSnapshot()})
	if err != nil {
		i.publish(SubmissionFailed{Raw: raw, Operation: "policy", Err: err})
		return
	}
	i.publish(SubmissionSucceeded{Raw: raw})
}
