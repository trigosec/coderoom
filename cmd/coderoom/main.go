// Command coderoom is the CLI entry point for Code Room.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
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

	factory := func(_ string, cwd string) agent.Agent {
		return codex.New(cwd)
	}

	var opts []ui.Option
	if *agentLog != "" {
		f, err := os.OpenFile(*agentLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agent-log: %v\n", err)
			return 1
		}
		defer func() {
			if err := f.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "agent-log close: %v\n", err)
			}
		}()
		factory = func(alias, cwd string) agent.Agent {
			return codex.New(cwd, codex.WithObserver(codex.NewLogObserver(f, alias)))
		}
	}
	opts = append(opts, ui.WithAgentFactory(factory))
	if strings.TrimSpace(os.Getenv("CODEROOM_DEBUG")) == "1" {
		opts = append(opts, ui.WithDebug(true))
	}

	if _, err := tea.NewProgram(
		ui.New(cwd, opts...),
		tea.WithAltScreen(),
	).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}
