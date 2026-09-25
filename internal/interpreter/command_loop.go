package interpreter

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/promptlang"
	roomstate "github.com/trigosec/coderoom/internal/room"
	"github.com/trigosec/coderoom/internal/session"
	"github.com/trigosec/coderoom/internal/shell"
)

type loopPhase uint8

const (
	loopWaitingForParticipant loopPhase = iota
	loopEvaluating
)

type loopExecution struct {
	statement promptlang.Loop
	body      promptlang.Shell
	phase     loopPhase
	turns     int
}

func (i *Interpreter) executeLoop(raw string, statement promptlang.Loop) {
	i.acceptInput(raw)
	if i.activeLoop != nil {
		i.publish(SubmissionFailed{Raw: raw, Operation: "loop", Code: ErrorExecutionFailed,
			Err: fmt.Errorf("a loop is already active")})
		return
	}
	body, err := i.commands.Resolve(promptlang.CommandInvocation{Name: statement.Condition})
	if err != nil {
		i.publish(SubmissionFailed{Raw: raw, Operation: "loop condition /" + statement.Condition,
			Code: ErrorExecutionFailed, Err: err})
		return
	}
	i.activeLoop = &loopExecution{statement: statement, body: body}
	i.sendLoopTurn(statement.Prompt)
	i.publish(SubmissionSucceeded{Raw: raw})
}

func (i *Interpreter) sendLoopTurn(prompt string) {
	loop := i.activeLoop
	plan := i.session.PlanSharedSend(loop.statement.Participant)
	err := i.session.Execute(session.SharedSendCommand{
		Plan:          plan,
		TextDirect:    prompt,
		TextListeners: fmt.Sprintf("@%s: %s", loop.statement.Participant, prompt),
	})
	i.drainSessionEvents(false)
	if err != nil && !slices.Contains(session.DeliveredAliases(err), loop.statement.Participant) {
		i.finishLoop("[loop] stopped: participant turn could not start")
		return
	}
	loop.turns++
	loop.phase = loopWaitingForParticipant
	i.publishLoopStatus(fmt.Sprintf("[loop] turn %d/%d sent to @%s",
		loop.turns, loop.statement.MaxTurns, loop.statement.Participant))
}

func (i *Interpreter) advanceLoop(event session.Event) {
	if i.activeLoop == nil || i.activeLoop.phase != loopWaitingForParticipant {
		return
	}
	alias := i.activeLoop.statement.Participant
	switch event := event.(type) {
	case session.ParticipantStatusChanged:
		if event.Alias == alias && event.To == participant.StatusIdle {
			i.startLoopCondition()
		}
	case session.AgentStopped:
		if event.Alias == alias {
			i.finishLoop("[loop] stopped: participant @" + alias + " stopped")
		}
	case session.AgentCrashed:
		if event.Alias == alias {
			i.finishLoop("[loop] stopped: participant @" + alias + " crashed")
		}
	}
}

func (i *Interpreter) startLoopCondition() {
	loop := i.activeLoop
	loop.phase = loopEvaluating
	i.shellWG.Add(1)
	go func(condition, program string) {
		defer i.shellWG.Done()
		result := i.runShell.Run(i.lifetime, i.cwd, program)
		i.enqueue(loopConditionCompletedOperation{condition: condition, result: result})
	}(loop.statement.Condition, loop.body.Program)
}

func (op loopConditionCompletedOperation) apply(i *Interpreter) {
	output := formatLoopConditionResult(op.result)
	i.room.AppendRecord(roomstate.NewAgentRecord(shellRecordAlias, agent.Message{
		Mode: agent.ModeSingle,
		Content: agent.Command{Command: "/" + op.condition, Cwd: i.cwd,
			Output: output, ExitCode: op.result.ExitCode},
	}))
	i.publish(ShellCompleted{Command: "/" + op.condition, Cwd: i.cwd, Result: op.result, Output: output})
	if i.activeLoop == nil || i.activeLoop.phase != loopEvaluating {
		return
	}
	switch op.result.Status {
	case shell.StatusSuccess:
		i.finishLoop("[loop] condition /" + op.condition + " succeeded")
	case shell.StatusCancelled:
		i.finishLoop("[loop] condition /" + op.condition + " cancelled")
	default:
		i.handleFailedLoopCondition(op.result)
	}
	i.publish(StateChanged{Snapshot: i.captureSnapshot()})
}

func (i *Interpreter) handleFailedLoopCondition(result shell.Result) {
	loop := i.activeLoop
	if loop.turns >= loop.statement.MaxTurns {
		i.finishLoop(fmt.Sprintf("[loop] reached /max %d; condition /%s still failing",
			loop.statement.MaxTurns, loop.statement.Condition))
		return
	}
	i.sendLoopTurn(formatLoopPrompt(loop.statement, result))
}

func (i *Interpreter) finishLoop(message string) {
	i.activeLoop = nil
	i.publishLoopStatus(message)
}

func (i *Interpreter) publishLoopStatus(message string) {
	i.room.AppendSystemRecord(message)
	i.publish(LoopStatus{Message: message})
}

func formatLoopPrompt(statement promptlang.Loop, result shell.Result) string {
	errorText := ""
	if result.Err != nil {
		errorText = result.Err.Error()
	}
	return strings.Join([]string{
		statement.Prompt, "",
		"The completion condition is failing. Continue working on the task using the evidence below.", "",
		"Condition command: /" + statement.Condition,
		"Status: " + string(result.Status),
		"Exit code: " + formatExitCode(result.ExitCode),
		"Stdout:\n" + formatEvidence(result.Stdout),
		"Stderr:\n" + formatEvidence(result.Stderr),
		"Error:\n" + formatEvidence(errorText),
	}, "\n")
}

func formatLoopConditionResult(result shell.Result) string {
	errorText := ""
	if result.Err != nil {
		errorText = result.Err.Error()
	}
	return strings.Join([]string{
		"status: " + string(result.Status),
		"exit code: " + formatExitCode(result.ExitCode),
		"stdout:\n" + formatEvidence(result.Stdout),
		"stderr:\n" + formatEvidence(result.Stderr),
		"error:\n" + formatEvidence(errorText),
	}, "\n")
}

func formatExitCode(exitCode *int) string {
	if exitCode == nil {
		return "(none)"
	}
	return strconv.Itoa(*exitCode)
}

func formatEvidence(text string) string {
	if text == "" {
		return "(none)"
	}
	return text
}
