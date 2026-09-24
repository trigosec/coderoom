package interpreter

import (
	"errors"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/session"
)

var (
	errInvalidApprovalChoice    = errors.New("invalid approval choice")
	errApprovalNotActive        = errors.New("approval is not active")
	errApprovalChoiceNotOffered = errors.New("approval choice was not offered")
)

type resolveApprovalOperation struct {
	id     int64
	choice ApprovalChoice
}

// ResolveApproval queues a structured response to the active approval.
func (i *Interpreter) ResolveApproval(id int64, choice ApprovalChoice) error {
	if !i.enqueue(resolveApprovalOperation{id: id, choice: choice}) {
		return ErrClosed
	}
	return nil
}

func (op resolveApprovalOperation) apply(i *Interpreter) {
	choice, err := i.approvalChoice(op.id, op.choice)
	if err != nil {
		i.publish(OperationFailed{Operation: "resolve approval", Err: err})
		return
	}
	err = i.session.Execute(session.ResolveApprovalCommand{ApprovalID: op.id, Choice: choice})
	if err != nil {
		i.publish(OperationFailed{Operation: "resolve approval", Err: err})
		return
	}
	i.clearApproval(op.id)
	i.publish(StateChanged{Snapshot: i.captureSnapshot()})
}

func agentApprovalChoice(choice ApprovalChoice) (agent.ApprovalOption, bool) {
	option := agent.ApprovalOption(choice.OptionID)
	switch option {
	case agent.OptionAccept, agent.OptionAcceptForSession, agent.OptionDecline, agent.OptionCancel:
		return option, true
	default:
		return "", false
	}
}

func (i *Interpreter) approvalChoice(id int64, choice ApprovalChoice) (agent.ApprovalOption, error) {
	i.stateMu.RLock()
	defer i.stateMu.RUnlock()
	if i.approval == nil || i.approval.ID != id {
		return "", errApprovalNotActive
	}
	for _, option := range i.approval.Options {
		if option.ID != choice.OptionID {
			continue
		}
		value, ok := agentApprovalChoice(choice)
		if !ok {
			return "", errInvalidApprovalChoice
		}
		return value, nil
	}
	return "", errApprovalChoiceNotOffered
}

func (i *Interpreter) clearApproval(id int64) bool {
	i.stateMu.Lock()
	defer i.stateMu.Unlock()
	if i.approval == nil || i.approval.ID != id {
		return false
	}
	i.approval = nil
	return true
}
