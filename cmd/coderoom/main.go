// Command coderoom is the CLI entry point for Code Room.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/trigosec/coderoom/internal/agent/codex"
	"github.com/trigosec/coderoom/internal/participant"
	"github.com/trigosec/coderoom/internal/session"
)

// printer is a session.Observer that writes events to stdout.
// The mutex ensures that multi-write sequences (alias prefix + delta text)
// are not interleaved when multiple agents stream simultaneously.
type printer struct {
	mu        sync.Mutex
	streaming map[string]bool // agents currently mid-stream
}

func (p *printer) OnEvent(e session.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch e.Kind {
	case session.KindAgentStarted:
		fmt.Printf("[%s joined]\n", e.Alias)
	case session.KindAgentStopped:
		p.endStream(e.Alias)
		fmt.Printf("[%s left]\n", e.Alias)
	case session.KindAgentCrashed:
		p.endStream(e.Alias)
		fmt.Printf("[%s crashed]\n", e.Alias)
	case session.KindBroadcast:
		fmt.Printf("[all] %s\n", e.Text)
	case session.KindSharedSend:
		fmt.Printf("[→ %s] %s\n", e.Alias, e.Text)
	case session.KindSharedNotice:
		fmt.Printf("[notice → %s]\n", e.Alias)
	case session.KindDelta:
		if !p.streaming[e.Alias] {
			fmt.Printf("%s> ", e.Alias)
			p.streaming[e.Alias] = true
		}
		fmt.Print(e.Text)
	case session.KindDone:
		p.endStream(e.Alias)
	}
}

func handleInvite(s *session.Session, cwd, line string) error {
	alias := strings.TrimSpace(strings.TrimPrefix(line, "/invite "))
	if alias == "" {
		return fmt.Errorf("usage: /invite <alias>")
	}
	if err := s.Execute(session.InviteCommand{
		Alias:      alias,
		Agent:      codex.New(cwd),
		Role:       participant.RoleBuilder,
		Initiative: participant.InitiativeManual,
	}); err != nil {
		return fmt.Errorf("invite %q: %w", alias, err)
	}
	return nil
}

func handleStop(s *session.Session, line string) error {
	alias := strings.TrimSpace(strings.TrimPrefix(line, "/stop "))
	if alias == "" {
		return fmt.Errorf("usage: /stop <alias>")
	}
	if err := s.Execute(session.StopCommand{Alias: alias}); err != nil {
		return fmt.Errorf("stop %q: %w", alias, err)
	}
	return nil
}

func handleSharedSend(s *session.Session, rest string) error {
	parts := strings.SplitN(rest, " ", 2)
	if len(parts) != 2 {
		fmt.Fprintln(os.Stderr, "usage: @<alias> <text>")
		return nil
	}
	alias, text := parts[0], parts[1]
	if err := s.Execute(session.SharedSendCommand{
		Alias:         alias,
		TextDirect:    text,
		TextListeners: fmt.Sprintf("@%s: %s", alias, text),
	}); err != nil {
		return fmt.Errorf("send to %q: %w", alias, err)
	}
	return nil
}

// endStream closes an in-progress stream line for alias, if any.
func (p *printer) endStream(alias string) {
	if p.streaming[alias] {
		fmt.Println()
		delete(p.streaming, alias)
	}
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
		os.Exit(1)
	}

	p := &printer{streaming: make(map[string]bool)}
	s := session.New(session.WithObserver(p))

	fmt.Println("coderoom ready.")
	fmt.Println("  /invite <alias>    invite a Codex agent")
	fmt.Println("  /stop <alias>      stop an agent")
	fmt.Println("  @<alias> <text>    send to one agent (shared room)")
	fmt.Println("  <text>             broadcast to all agents")
	fmt.Println("  /quit              exit")

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if err := dispatch(s, cwd, line); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "stdin: %v\n", err)
	}
}

func dispatch(s *session.Session, cwd, line string) error {
	switch {
	case line == "/quit":
		s.Shutdown()
		fmt.Println("bye")
		os.Exit(0)

	case strings.HasPrefix(line, "/invite "):
		return handleInvite(s, cwd, line)

	case strings.HasPrefix(line, "/stop "):
		return handleStop(s, line)

	case strings.HasPrefix(line, "@"):
		return handleSharedSend(s, line[1:])

	default:
		if err := s.Execute(session.BroadcastCommand{Text: line}); err != nil {
			return fmt.Errorf("broadcast: %w", err)
		}
	}
	return nil
}
