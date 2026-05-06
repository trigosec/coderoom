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

const timeout = 30 * time.Second

func main() {
	prompt := "Say hello in exactly one sentence."
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}

	cmd := exec.Command("claude",
		"--print",
		"--verbose",
		"--output-format", "stream-json",
		prompt,
	)

	fmt.Fprintf(os.Stderr, "prompt: %q\n", prompt)
	fmt.Fprintln(os.Stderr, "starting claude...")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pty start: %v\n", err)
		os.Exit(1)
	}
	defer ptmx.Close()

	fmt.Fprintln(os.Stderr, "process started, waiting for output...")

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

	for {
		select {
		case line, ok := <-lines:
			if !ok {
				fmt.Fprintln(os.Stderr, "output stream closed")
				return
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "{") {
				continue
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				fmt.Fprintf(os.Stderr, "invalid json: %s\n", line)
				continue
			}
			b, _ := json.MarshalIndent(event, "", "  ")
			fmt.Println(string(b))
		case <-timer.C:
			fmt.Fprintln(os.Stderr, "timeout waiting for response")
			os.Exit(1)
		}
	}
}
