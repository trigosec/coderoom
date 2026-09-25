package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/interpreter"
	"github.com/trigosec/coderoom/internal/shell"
)

const shellRecordAlias = "shell"

func (m Model) appendShellResult(event interpreter.ShellCompleted) Model {
	m.room = m.room.AppendCommand(shellRecordAlias, agent.Command{
		Command:  event.Command,
		Cwd:      event.Cwd,
		Output:   event.Output,
		ExitCode: event.Result.ExitCode,
	})
	return m
}

// executeShellProgram remains temporarily for UI-owned bounded-loop condition
// evaluation. It moves with the loop workflow in the next migration step.
func (m Model) executeShellProgram(program string, message func(shell.Result) tea.Msg) tea.Cmd {
	run := m.runShell
	executions := m.executions
	cwd := m.cwd
	return func() tea.Msg {
		ctx, finish, err := executions.start()
		if err != nil {
			return message(shell.Result{Status: shell.StatusCancelled, Err: err})
		}
		defer finish()
		return message(run(ctx, cwd, program))
	}
}
