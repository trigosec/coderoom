package ui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/promptlang"
	"github.com/trigosec/coderoom/internal/session"
	"github.com/trigosec/coderoom/internal/shell"
)

type loopPhase uint8

const (
	loopEvaluating loopPhase = iota
	loopWaitingForParticipant
)

type loopExecution struct {
	statement promptlang.Loop
	body      promptlang.Shell
	phase     loopPhase
	turns     int
}

type loopConditionResultMsg struct {
	condition string
	cwd       string
	result    shell.Result
}

func (m Model) startLoop(statement promptlang.Loop) Model {
	if m.activeLoop != nil {
		m.room = m.room.AppendSystem("error: a loop is already active")
		return m
	}
	body, err := m.commands.Resolve(promptlang.CommandInvocation{Name: statement.Condition})
	if err != nil {
		m.room = m.room.AppendSystem(fmt.Sprintf("error: loop condition /%s: %v", statement.Condition, err))
		return m
	}
	m.activeLoop = &loopExecution{statement: statement, body: body}
	return m.sendLoopTurn(statement.Prompt)
}

func (m Model) evaluateLoopCondition() tea.Cmd {
	loop := m.activeLoop
	return m.executeShellProgram(loop.body.Program, func(result shell.Result) tea.Msg {
		return loopConditionResultMsg{
			condition: loop.statement.Condition,
			cwd:       m.cwd,
			result:    result,
		}
	})
}

func (m Model) handleLoopConditionResult(msg loopConditionResultMsg) (Model, tea.Cmd) {
	m = m.handleShellResult(shellResultMsg{
		command: "/" + msg.condition,
		cwd:     msg.cwd,
		result:  msg.result,
	})
	if m.activeLoop == nil || m.activeLoop.phase != loopEvaluating {
		return m, nil
	}
	switch msg.result.Status {
	case shell.StatusSuccess:
		return m.finishLoop("[loop] condition /" + msg.condition + " succeeded"), nil
	case shell.StatusCancelled:
		return m.finishLoop("[loop] condition /" + msg.condition + " cancelled"), nil
	default:
		return m.handleFailedLoopCondition(msg.result)
	}
}

func (m Model) handleFailedLoopCondition(result shell.Result) (Model, tea.Cmd) {
	loop := m.activeLoop
	if loop.turns >= loop.statement.MaxTurns {
		message := fmt.Sprintf("[loop] reached /max %d; condition /%s still failing",
			loop.statement.MaxTurns, loop.statement.Condition)
		return m.finishLoop(message), nil
	}

	return m.sendLoopTurn(formatLoopPrompt(loop.statement, result)), nil
}

func (m Model) sendLoopTurn(prompt string) Model {
	loop := m.activeLoop
	next, delivered, err := m.executeSendToAgent(loop.statement.Participant, prompt)
	m = next
	if err != nil && !slices.Contains(delivered, loop.statement.Participant) {
		return m.finishLoop("[loop] stopped: participant turn could not start")
	}
	loop.turns++
	loop.phase = loopWaitingForParticipant
	m.room = m.room.AppendSystem(fmt.Sprintf("[loop] turn %d/%d sent to @%s",
		loop.turns, loop.statement.MaxTurns, loop.statement.Participant))
	return m
}

func formatLoopPrompt(statement promptlang.Loop, result shell.Result) string {
	return fmt.Sprintf("%s\n\nCondition command: /%s\n%s",
		statement.Prompt, statement.Condition, formatShellResult(result))
}

func (m Model) advanceLoopForEvent(event session.Event) (Model, tea.Cmd) {
	if m.activeLoop == nil || m.activeLoop.phase != loopWaitingForParticipant {
		return m, nil
	}
	alias := m.activeLoop.statement.Participant
	switch event := event.(type) {
	case session.ParticipantStatusChanged:
		if event.Alias != alias || event.To != participant.StatusIdle {
			return m, nil
		}
		m.activeLoop.phase = loopEvaluating
		return m, m.evaluateLoopCondition()
	case session.AgentStopped:
		if event.Alias == alias {
			return m.finishLoop("[loop] stopped: participant @" + alias + " stopped"), nil
		}
	case session.AgentCrashed:
		if event.Alias == alias {
			return m.finishLoop("[loop] stopped: participant @" + alias + " crashed"), nil
		}
	}
	return m, nil
}

func (m Model) finishLoop(message string) Model {
	m.activeLoop = nil
	m.room = m.room.AppendSystem(message)
	return m
}
