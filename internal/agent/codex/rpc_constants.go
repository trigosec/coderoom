package codex

const (
	methodInitialize    = "initialize"
	methodThreadStart   = "thread/start"
	methodTurnStart     = "turn/start"
	methodTurnStarted   = "turn/started"
	methodTurnCompleted = "turn/completed"
	methodTurnFailed    = "turn/failed"
	methodTurnInterrupt = "turn/interrupt"

	methodAgentDelta = "item/agentMessage/delta"

	methodReasoningTextDelta        = "item/reasoning/textDelta"
	methodReasoningSummaryTextDelta = "item/reasoning/summaryTextDelta"
	methodReasoningSummaryPartAdded = "item/reasoning/summaryPartAdded"

	methodCommandExecutionRequestApproval = "item/commandExecution/requestApproval"
	methodFileChangeRequestApproval       = "item/fileChange/requestApproval"
	methodPermissionsRequestApproval      = "item/permissions/requestApproval"
)
