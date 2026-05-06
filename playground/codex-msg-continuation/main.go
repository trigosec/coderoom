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

var turns = []string{
	"What is 2 + 2?",
	"Multiply that result by 3.",
}

func main() {
	var result string
	var err error

	for i, turn := range turns {
		fmt.Fprintf(os.Stderr, "\n--- turn %d: %q ---\n", i+1, turn)
		isFirst := i == 0
		result, err = runTurn(turn, isFirst)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	fmt.Fprintf(os.Stderr, "\n--- validation ---\n")
	// result is item.text from the item.completed event, not the full JSON,
	// so contains is safe here: other fields are not included.
	if strings.Contains(result, "12") {
		fmt.Fprintln(os.Stderr, "PASS: result contains 12, context preserved across turns")
	} else {
		fmt.Fprintf(os.Stderr, "FAIL: expected \"12\" in result, got: %s\n", result)
		os.Exit(1)
	}
}

func runTurn(prompt string, first bool) (string, error) {
	var args []string
	if first {
		args = []string{"@openai/codex", "exec", "--json", prompt}
	} else {
		// Session ID is not emitted in the --json output stream; use --last to
		// resume the most recent session without needing to parse the filesystem.
		args = []string{"@openai/codex", "exec", "resume", "--last", "--json", prompt}
	}

	cmd := exec.Command("npx", args...)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", fmt.Errorf("pty start: %w", err)
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

	return collectUntilResult(lines)
}

func collectUntilResult(lines <-chan string) (string, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var result string

	for {
		select {
		case line, ok := <-lines:
			if !ok {
				if result == "" {
					return "", fmt.Errorf("stream closed before result")
				}
				return result, nil
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

			// Response text is at item.text in the item.completed event.
			if event["type"] == "item.completed" {
				if item, ok := event["item"].(map[string]any); ok {
					if text, ok := item["text"].(string); ok {
						result = text
					}
				}
			}
		case <-timer.C:
			return "", fmt.Errorf("timeout waiting for result")
		}
	}
}
