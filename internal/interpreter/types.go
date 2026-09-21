package interpreter

import (
	"errors"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
	roomstate "github.com/trigosec/coderoom/internal/room"
)

// ErrStagePending rejects new input while a staged batch awaits a stage action.
var ErrStagePending = errors.New("submission blocked by pending stage")

// ApprovalKind identifies an approval request without exposing agent protocol
// types to front ends.
type ApprovalKind string

// Supported approval request kinds.
const (
	ApprovalCommandExecution ApprovalKind = "commandExecution"
	ApprovalFileChange       ApprovalKind = "fileChange"
	ApprovalPermissions      ApprovalKind = "permissions"
)

// ApprovalOption is one selectable approval response.
type ApprovalOption struct {
	ID    string
	Label string
}

// Approval is the active application-level approval request.
type Approval struct {
	ID      int64
	Alias   string
	Kind    ApprovalKind
	Prompt  string
	Options []ApprovalOption
}

// ApprovalChoice identifies the option selected by a front end.
type ApprovalChoice struct{ OptionID string }

// Snapshot is a detached point-in-time view of interpreter state.
type Snapshot struct {
	Room         roomstate.Snapshot
	Participants []participant.View
	Approval     *Approval
}

// Event is an application event emitted by the interpreter.
type Event interface{ interpreterEvent() }

// StateChanged reports a new immutable application snapshot.
type StateChanged struct{ Snapshot Snapshot }

// OperationFailed reports an asynchronous interpreter operation failure.
type OperationFailed struct {
	Operation string
	Err       error
}

// InputAccepted reports prompt-language input accepted for execution.
type InputAccepted struct {
	Raw     string
	Routing []string
}

// InputRejected reports input rejected before acceptance.
type InputRejected struct {
	Raw string
	Err error
}

// UnknownCommand reports valid input with no interpreter handler or fallback.
type UnknownCommand struct {
	Raw  string
	Name string
}

func (StateChanged) interpreterEvent()    {}
func (OperationFailed) interpreterEvent() {}
func (InputAccepted) interpreterEvent()   {}
func (InputRejected) interpreterEvent()   {}
func (UnknownCommand) interpreterEvent()  {}

// Observer consumes application events. Implementations should return quickly.
type Observer interface{ OnEvent(Event) }

func approvalFromAgent(id int64, alias string, request agent.ApprovalRequest) Approval {
	options := make([]ApprovalOption, len(request.Options))
	for index, option := range request.Options {
		options[index] = ApprovalOption{ID: string(option), Label: approvalOptionLabel(option)}
	}
	return Approval{
		ID:      id,
		Alias:   alias,
		Kind:    ApprovalKind(request.Kind),
		Prompt:  request.Ask,
		Options: options,
	}
}

func approvalOptionLabel(option agent.ApprovalOption) string {
	switch option {
	case agent.OptionAccept:
		return "Accept"
	case agent.OptionAcceptForSession:
		return "Accept for session"
	case agent.OptionDecline:
		return "Decline"
	case agent.OptionCancel:
		return "Cancel"
	default:
		return string(option)
	}
}
