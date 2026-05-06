package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
)

const timeout = 60 * time.Second

func main() {
	cmd := exec.Command("npx", "@openai/codex", "exec", "--json", "Say hello in one sentence.")
	cmd.Env = append(os.Environ(), "CODEX_QUIET_MODE=1")

	fmt.Fprintln(os.Stderr, "process started (CODEX_QUIET_MODE=1)")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pty start: %v\n", err)
		os.Exit(1)
	}
	defer ptmx.Close()

	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(ptmx)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var gotResult bool
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				if !gotResult {
					fmt.Fprintln(os.Stderr, "FAIL: stream closed before result")
					os.Exit(1)
				}
				return
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "{") {
				continue
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			b, _ := json.MarshalIndent(event, "", "  ")
			fmt.Println(string(b))

			// Inspect thread.started for session metadata to check config loading.
			if event["type"] == "thread.started" {
				validate(event)
			}
			if t, _ := event["type"].(string); t == "turn.completed" || t == "error" {
				gotResult = true
				fmt.Fprintf(os.Stderr, "\nfinal event type: %s\n", t)
			}
		case <-timer.C:
			fmt.Fprintln(os.Stderr, "timeout waiting for response")
			os.Exit(1)
		}
	}
}

// validate inspects the thread.started event for signs of user config loading.
// Field paths are best guesses; adjust once actual event structure is observed.
func validate(event map[string]any) {
	fmt.Fprintln(os.Stderr, "\n--- thread.started validation ---")
	b, _ := json.MarshalIndent(event, "", "  ")
	fmt.Fprintf(os.Stderr, "%s\n", b)
}
