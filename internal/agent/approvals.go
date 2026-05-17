package agent

import "context"

// ApprovalListener receives normalized approval requests from an agent backend
// and returns a normalized decision. ctx is cancelled when the agent shuts down;
// implementations should respect it to avoid blocking indefinitely.
type ApprovalListener interface {
	Decide(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error)
}

// ApprovalKind identifies the type of approval requested.
type ApprovalKind string

const (
	// ApprovalCommandExecution indicates an approval request for running a command.
	ApprovalCommandExecution ApprovalKind = "commandExecution"
	// ApprovalFileChange indicates an approval request for writing files / applying file changes.
	ApprovalFileChange ApprovalKind = "fileChange"
	// ApprovalPermissions indicates an approval request for granting additional permissions.
	ApprovalPermissions ApprovalKind = "permissions"
)

// ApprovalRequest is a normalized, user-facing approval prompt.
type ApprovalRequest struct {
	Kind ApprovalKind

	// Ask is intended to be rendered directly.
	Ask string

	// Options is the set of valid choices for this request.
	Options []ApprovalOption
}

// ApprovalOption is a normalized approval response choice.
type ApprovalOption string

const (
	// OptionAccept approves the requested action.
	OptionAccept ApprovalOption = "accept"
	// OptionAcceptForSession approves the requested action and caches it for the session when supported.
	OptionAcceptForSession ApprovalOption = "acceptForSession"
	// OptionDecline denies the requested action.
	OptionDecline ApprovalOption = "decline"
	// OptionCancel denies the requested action and interrupts the current turn when supported.
	OptionCancel ApprovalOption = "cancel"
)

// ApprovalDecision is a normalized approval outcome from an ApprovalListener.
type ApprovalDecision struct {
	Choice ApprovalOption
}
