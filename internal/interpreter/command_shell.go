package interpreter

import (
	"context"
	"strings"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/promptlang"
	roomstate "github.com/trigosec/coderoom/internal/room"
	"github.com/trigosec/coderoom/internal/shell"
)

const shellRecordAlias = "shell"

// ShellRunner executes one shell program.
type ShellRunner interface {
	Run(context.Context, string, string) shell.Result
}

// ShellRunnerFunc adapts a function to ShellRunner.
type ShellRunnerFunc func(context.Context, string, string) shell.Result

// Run executes the adapted function.
func (f ShellRunnerFunc) Run(ctx context.Context, cwd, program string) shell.Result {
	return f(ctx, cwd, program)
}

// WithShellRunner replaces local shell execution, primarily for tests.
func WithShellRunner(runner ShellRunner) Option {
	return func(i *Interpreter) {
		if runner != nil {
			i.runShell = runner
		}
	}
}

func (i *Interpreter) executeCommandDefinition(raw string, definition promptlang.CommandDefinition) {
	i.acceptInput(raw)
	if err := i.commands.Define(definition); err != nil {
		i.publish(SubmissionFailed{
			Raw:       raw,
			Operation: "define /" + definition.Name,
			Code:      submissionErrorCode(err),
			Err:       err,
		})
		return
	}
	i.room.AppendSystemRecord("[defined] /" + definition.Name)
	i.publish(StateChanged{Snapshot: i.captureSnapshot()})
	i.publish(SubmissionSucceeded{Raw: raw})
}

func (i *Interpreter) executeCommandInvocation(raw string, invocation promptlang.CommandInvocation) {
	body, err := i.commands.Resolve(invocation)
	if err != nil {
		i.publish(UnknownCommand{Raw: raw, Name: invocation.Name})
		return
	}
	i.acceptInput(raw)
	i.startShell(raw, "/"+invocation.Name, body.Program)
}

func (i *Interpreter) executeShell(raw, command, program string) {
	i.acceptInput(raw)
	i.startShell(raw, command, program)
}

func (i *Interpreter) acceptInput(raw string) {
	i.room.AppendUserInputRecord(raw, nil)
	i.publish(InputAccepted{Raw: raw})
}

func (i *Interpreter) startShell(raw, command, program string) {
	i.shellWG.Add(1)
	go func() {
		defer i.shellWG.Done()
		result := i.runShell.Run(i.lifetime, i.cwd, program)
		i.enqueue(shellCompletedOperation{command: command, result: result})
	}()
	i.publish(SubmissionSucceeded{Raw: raw})
}

func (op shellCompletedOperation) apply(i *Interpreter) {
	i.room.AppendRecord(roomstate.NewAgentRecord(shellRecordAlias, agent.Message{
		Mode: agent.ModeSingle,
		Content: agent.Command{
			Command:  op.command,
			Cwd:      i.cwd,
			Output:   formatShellResult(op.result),
			ExitCode: op.result.ExitCode,
		},
	}))
	i.publish(ShellCompleted{
		Command: op.command,
		Cwd:     i.cwd,
		Result:  op.result,
		Output:  formatShellResult(op.result),
	})
	i.publish(StateChanged{Snapshot: i.captureSnapshot()})
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
