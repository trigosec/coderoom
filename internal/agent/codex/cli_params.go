package codex

// AskForApprovalPolicy configures when Codex asks for human approval before
// executing a command.
type AskForApprovalPolicy string

const (
	// AskDefault means "do not pass --ask-for-approval"; Codex will use its own default.
	AskDefault AskForApprovalPolicy = ""
	// AskUntrusted runs "trusted" commands without approval and asks for others.
	AskUntrusted AskForApprovalPolicy = "untrusted"
	// AskOnFailure runs commands without approval and only asks when a command fails.
	AskOnFailure AskForApprovalPolicy = "on-failure"
	// AskOnRequest lets the model decide when to request approval.
	AskOnRequest AskForApprovalPolicy = "on-request"
	// AskNever disables approval prompts.
	AskNever AskForApprovalPolicy = "never"
)

// SandboxMode selects the sandbox policy Codex uses when executing
// model-generated shell commands.
type SandboxMode string

const (
	// SandboxDefault means "do not pass --sandbox"; Codex will use its own default.
	SandboxDefault SandboxMode = ""
	// SandboxReadOnly prevents the agent from writing files.
	SandboxReadOnly SandboxMode = "read-only"
	// SandboxWorkspaceWrite allows writes only within the workspace and writable roots.
	SandboxWorkspaceWrite SandboxMode = "workspace-write"
	// SandboxDangerFull gives the agent broad file access (use with care).
	SandboxDangerFull SandboxMode = "danger-full-access"
)

// ReasoningEffort configures Codex's model_reasoning_effort setting.
type ReasoningEffort string

const (
	// ReasoningDefault means "do not pass model_reasoning_effort"; Codex uses its default.
	ReasoningDefault ReasoningEffort = ""
	// ReasoningMinimal asks Codex for minimal reasoning effort.
	ReasoningMinimal ReasoningEffort = "minimal"
	// ReasoningLow asks Codex for low reasoning effort.
	ReasoningLow ReasoningEffort = "low"
	// ReasoningMedium asks Codex for medium reasoning effort.
	ReasoningMedium ReasoningEffort = "medium"
	// ReasoningHigh asks Codex for high reasoning effort.
	ReasoningHigh ReasoningEffort = "high"
	// ReasoningXHigh asks Codex for extra-high reasoning effort.
	ReasoningXHigh ReasoningEffort = "xhigh"
)

// ReasoningSummary configures Codex's model_reasoning_summary setting.
type ReasoningSummary string

const (
	// ReasoningSummaryDefault means "do not pass model_reasoning_summary".
	ReasoningSummaryDefault ReasoningSummary = ""
	// ReasoningSummaryAuto lets Codex choose the reasoning summary level.
	ReasoningSummaryAuto ReasoningSummary = "auto"
	// ReasoningSummaryConcise asks Codex for concise reasoning summaries.
	ReasoningSummaryConcise ReasoningSummary = "concise"
	// ReasoningSummaryDetailed asks Codex for detailed reasoning summaries.
	ReasoningSummaryDetailed ReasoningSummary = "detailed"
	// ReasoningSummaryNone disables reasoning summaries.
	ReasoningSummaryNone ReasoningSummary = "none"
)
