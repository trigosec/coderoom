package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/shell"
)

const shellRecordAlias = "shell"

type shellResultMsg struct {
	command string
	cwd     string
	result  shell.Result
}

func (m Model) executeShell(program string) tea.Cmd {
	return m.executeShellCommand(program, program)
}

func (m Model) executeShellCommand(command, program string) tea.Cmd {
	return m.executeShellProgram(program, func(result shell.Result) tea.Msg {
		return shellResultMsg{command: command, cwd: m.cwd, result: result}
	})
}

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

func (m Model) handleShellResult(msg shellResultMsg) Model {
	return m.appendShellResult(msg, formatShellResult(msg.result))
}

func (m Model) appendShellResult(msg shellResultMsg, output string) Model {
	m.room = m.room.AppendCommand(shellRecordAlias, agent.Command{
		Command:  msg.command,
		Cwd:      msg.cwd,
		Output:   output,
		ExitCode: msg.result.ExitCode,
	})
	return m
}

func formatShellResult(result shell.Result) string {
	sections := []string{"status: " + string(result.Status)}
	if result.Stdout != "" {
		sections = append(sections, "stdout:\n"+result.Stdout)
	}
	if result.Stderr != "" {
		sections = append(sections, "stderr:\n"+result.Stderr)
	}
	if result.Err != nil {
		sections = append(sections, "error:\n"+result.Err.Error())
	}
	return strings.Join(sections, "\n")
}
