package ui

import (
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/interpreter"
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
