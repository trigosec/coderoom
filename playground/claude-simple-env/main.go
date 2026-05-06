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
	cmd := exec.Command("claude",
		"--print",
		"--verbose",
		"--output-format", "stream-json",
		"Say hello in one sentence.",
	)
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_SIMPLE=1")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pty start: %v\n", err)
		os.Exit(1)
	}
	defer ptmx.Close()

	fmt.Fprintln(os.Stderr, "process started (CLAUDE_CODE_SIMPLE=1)")

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
			if event["type"] == "system" && event["subtype"] == "init" {
				validate(event)
			}
			if event["type"] == "result" {
				gotResult = true
				fmt.Fprintln(os.Stderr, "\nPASS: got result, process completed successfully")
			}
		case <-timer.C:
			fmt.Fprintln(os.Stderr, "timeout waiting for result")
			os.Exit(1)
		}
	}
}

// validate inspects the system init event and reports whether personal config
// (plugins, MCP servers, skills) is present or suppressed.
func validate(event map[string]any) {
	fmt.Fprintln(os.Stderr, "\n--- system init validation ---")

	plugins, _ := event["plugins"].([]any)
	mcpServers, _ := event["mcp_servers"].([]any)
	skills, _ := event["skills"].([]any)

	fmt.Fprintf(os.Stderr, "plugins:     %d (want 0)\n", len(plugins))
	fmt.Fprintf(os.Stderr, "mcp_servers: %d (want 0)\n", len(mcpServers))
	fmt.Fprintf(os.Stderr, "skills:      %d (want 0)\n", len(skills))

	if len(plugins) == 0 && len(mcpServers) == 0 && len(skills) == 0 {
		fmt.Fprintln(os.Stderr, "PASS: personal config suppressed")
	} else {
		fmt.Fprintln(os.Stderr, "FAIL: personal config still present")
	}
}
