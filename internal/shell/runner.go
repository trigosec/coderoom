// Package shell executes shell programs for the Code Room prompt language.
package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	executable    = "/bin/sh"
	pipeWaitDelay = 250 * time.Millisecond
)

// Status describes how a shell program finished.
type Status string

const (
	// StatusSuccess means the shell program exited normally with code zero.
	StatusSuccess Status = "success"

	// StatusFailure means the program failed or the runner could not complete it.
	StatusFailure Status = "failure"

	// StatusCancelled means context cancellation stopped the shell program.
	StatusCancelled Status = "cancelled"
)

// Result contains the observable result of a shell program.
type Result struct {
	Status   Status
	ExitCode *int
	Stdout   string
	Stderr   string
	Err      error
}

// Run executes program with /bin/sh in cwd. The process inherits the current
// environment. Cancelling ctx terminates the shell process.
func Run(ctx context.Context, cwd, program string) Result {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newCommand(ctx, program)
	cmd.Dir = cwd
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	cleanupErr := killProcessGroup(cmd)
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return successfulResult(result, cmd.ProcessState, cleanupErr)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		result.Status = StatusCancelled
		result.Err = errors.Join(ctxErr, cleanupErr)
		return result
	}
	if isSuccessfulPipeWait(err, cmd.ProcessState) {
		return successfulResult(result, cmd.ProcessState, cleanupErr)
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.Status = StatusFailure
		result.ExitCode = normalExitCode(exitErr.ProcessState)
		result.Err = cleanupErr
		return result
	}

	result.Status = StatusFailure
	result.Err = fmt.Errorf("start shell: %w", err)
	return result
}

func successfulResult(result Result, state *os.ProcessState, cleanupErr error) Result {
	if cleanupErr != nil {
		result.Status = StatusFailure
		result.Err = fmt.Errorf("clean up shell process group: %w", cleanupErr)
		return result
	}
	result.Status = StatusSuccess
	result.ExitCode = normalExitCode(state)
	return result
}

func newCommand(ctx context.Context, program string) *exec.Cmd {
	//nolint:gosec // Running the explicit user-authored program is this package's purpose.
	cmd := exec.CommandContext(ctx, executable, "-c", program)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killProcessGroup(cmd) }
	cmd.WaitDelay = pipeWaitDelay
	return cmd
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return fmt.Errorf("kill shell process group: %w", err)
}

func isSuccessfulPipeWait(err error, state *os.ProcessState) bool {
	return errors.Is(err, exec.ErrWaitDelay) && state != nil && state.Success()
}

func normalExitCode(state *os.ProcessState) *int {
	if state == nil {
		return nil
	}
	waitStatus, ok := state.Sys().(syscall.WaitStatus)
	if ok && !waitStatus.Exited() {
		return nil
	}
	exitCode := state.ExitCode()
	if exitCode < 0 {
		return nil
	}
	return &exitCode
}
