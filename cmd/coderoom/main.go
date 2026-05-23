// Command coderoom is the CLI entry point for Code Room.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
	"github.com/trigosec/coderoom/internal/session"
	"github.com/trigosec/coderoom/internal/ui"
)

func main() {
	os.Exit(run())
}

func run() int {
	agentLog := flag.String("agent-log", "", "write raw agent JSON-RPC traffic to `file`")
	flag.Parse()

	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected arguments: %v\n", flag.Args())
		flag.Usage()
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	factory := buildFactory(ctx, cwd, *agentLog)
	if factory == nil {
		return 1
	}
	if factory.cleanup != nil {
		defer factory.cleanup()
	}

	sess := session.New(session.WithAgentFactory(factory.agentFactory))

	var opts []ui.Option
	if strings.TrimSpace(os.Getenv("CODEROOM_DEBUG")) == "1" {
		opts = append(opts, ui.WithDebug(true))
	}
	opts = append(opts, ui.WithStartupHelpTip(true))

	if _, err := tea.NewProgram(
		ui.New(sess, cwd, opts...),
		tea.WithAltScreen(),
	).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

type builtFactory struct {
	agentFactory session.AgentFactory
	cleanup      func()
}

func buildFactory(ctx context.Context, cwd, agentLog string) *builtFactory {
	if agentLog == "" {
		return &builtFactory{
			agentFactory: func(_ string) agent.Agent {
				return codex.New(cwd, codex.WithContext(ctx))
			},
		}
	}

	f, err := os.OpenFile(filepath.Clean(agentLog), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent-log: %v\n", err)
		return nil
	}
	return &builtFactory{
		agentFactory: func(alias string) agent.Agent {
			return codex.New(cwd, codex.WithContext(ctx), codex.WithObserver(codex.NewLogObserver(f, alias)))
		},
		cleanup: func() {
			if err := f.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "agent-log close: %v\n", err)
			}
		},
	}
}
