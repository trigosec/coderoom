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
	var sessionID, result string
	var err error

	for i, turn := range turns {
		fmt.Fprintf(os.Stderr, "\n--- turn %d: %q ---\n", i+1, turn)
		sessionID, result, err = runTurn(turn, sessionID)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "session_id: %s\n", sessionID)
	}

	fmt.Fprintf(os.Stderr, "\n--- validation ---\n")
	// result is the value of the "result" field in the final event, not the full message,
	// so contains is safe here: other fields (session_id, token counts, etc.) are not included.
	if strings.Contains(result, "12") {
		fmt.Fprintln(os.Stderr, "PASS: result contains 12, context preserved across turns")
	} else {
		fmt.Fprintf(os.Stderr, "FAIL: expected \"12\" in result, got: %s\n", result)
		os.Exit(1)
	}
}

func runTurn(prompt, sessionID string) (string, string, error) {
	args := []string{
		"--print",
		"--verbose",
		"--output-format", "stream-json",
		prompt,
	}
	if sessionID != "" {
		args = append([]string{"--resume", sessionID}, args...)
	}

	cmd := exec.Command("claude", args...)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", "", fmt.Errorf("pty start: %w", err)
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

func collectUntilResult(lines <-chan string) (string, string, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return "", "", fmt.Errorf("stream closed before result")
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
			if event["type"] == "result" {
				sid, _ := event["session_id"].(string)
				result, _ := event["result"].(string)
				return sid, result, nil
			}
		case <-timer.C:
			return "", "", fmt.Errorf("timeout waiting for result")
		}
	}
}
