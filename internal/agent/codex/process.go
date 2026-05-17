package codex

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type process struct {
	cwd string
	cmd *exec.Cmd

	codexIn  io.WriteCloser
	codexOut *bufio.Reader
	codexErr io.Reader

	askForApproval AskForApprovalPolicy
	sandboxMode    SandboxMode
}

func newProc(cwd string) *process {
	return &process{cwd: cwd, askForApproval: AskDefault, sandboxMode: SandboxDefault}
}

func (p *process) start() error {
	args := codexArgs(p.askForApproval, p.sandboxMode)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = p.cwd

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("codex stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("codex stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("codex stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("codex start: %w", err)
	}

	p.codexIn = stdin
	p.codexOut = bufio.NewReader(stdout)
	p.codexErr = stderr
	p.cmd = cmd
	return nil
}

// codexArgs returns the command and arguments for the Codex app-server.
// CODEX_VERSION_OVERRIDE pins a specific npm version for integration testing.
func codexArgs(askForApproval AskForApprovalPolicy, sandboxMode SandboxMode) []string {
	pkg := "@openai/codex"
	if v := os.Getenv("CODEX_VERSION_OVERRIDE"); v != "" {
		pkg = "@openai/codex@" + v
	}
	args := []string{"npx", pkg}
	if askForApproval != AskDefault {
		args = append(args, "--ask-for-approval", string(askForApproval))
	}
	if sandboxMode != SandboxDefault {
		args = append(args, "--sandbox", string(sandboxMode))
	}
	args = append(args, "app-server")
	return args
}
