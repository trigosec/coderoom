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

	tea "charm.land/bubbletea/v2"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
	"github.com/trigosec/coderoom/internal/config"
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

	cleanup, factoryOpt, err := agentFactoryOption(cwd, *agentLog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent factory: %v\n", err)
		return 1
	}
	if cleanup != nil {
		defer cleanup()
	}
	cfg := config.New(cwd)
	sess := session.New(session.WithContext(ctx), session.WithConfig(cfg), factoryOpt)

	var opts []ui.Option
	if strings.TrimSpace(os.Getenv("CODEROOM_DEBUG")) == "1" {
		opts = append(opts, ui.WithDebug(true))
	}
	opts = append(opts, ui.WithStartupHelpTip(true))

	if _, err := tea.NewProgram(
		ui.New(sess, cwd, opts...),
	).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func agentFactoryOption(cwd, agentLog string) (cleanup func(), opt session.Option, err error) {
	if agentLog == "" {
		return nil, session.WithAgentFactory(func(s *session.Session, alias string) agent.Agent {
			return codex.New(
				cwd,
				codex.WithContext(s.CreateAgentContext(alias)),
				codex.WithApprovalListener(s.ApprovalListener(alias)),
			)
		}), nil
	}

	f, err := os.OpenFile(filepath.Clean(agentLog), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open agent log %q: %w", agentLog, err)
	}
	return func() {
			if err := f.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "agent-log close: %v\n", err)
			}
		}, session.WithAgentFactory(func(s *session.Session, alias string) agent.Agent {
			return codex.New(
				cwd,
				codex.WithContext(s.CreateAgentContext(alias)),
				codex.WithObserver(codex.NewLogObserver(f, alias)),
				codex.WithApprovalListener(s.ApprovalListener(alias)),
			)
		}), nil
}
