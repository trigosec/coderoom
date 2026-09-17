package interpreter

import (
	"errors"

	"github.com/trigosec/coderoom/internal/agent"
)

var (
	errInvalidApprovalChoice    = errors.New("invalid approval choice")
	errApprovalNotActive        = errors.New("approval is not active")
	errApprovalChoiceNotOffered = errors.New("approval choice was not offered")
)

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
