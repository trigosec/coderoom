package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const timeout = 60 * time.Second

var turns = []string{
	"What is 2 + 2?",
	"Multiply that result by 3.",
}

// rpcMsg covers requests, responses, and notifications.
// Requests and notifications have Method set.
// Responses have ID set with Result or Error.
type rpcMsg struct {
	Method string          `json:"method,omitempty"`
	ID     *int            `json:"id,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

var msgID int

func nextID() int {
	msgID++
	return msgID
}

func main() {
	cmd := exec.Command("npx", "@openai/codex", "app-server")
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdin pipe: %v\n", err)
		os.Exit(1)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdout pipe: %v\n", err)
		os.Exit(1)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "server start: %v\n", err)
		os.Exit(1)
	}
	defer cmd.Process.Kill()
	fmt.Fprintln(os.Stderr, "app-server started")

	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	send := func(method string, params any) (int, error) {
		id := nextID()
		b, _ := json.Marshal(map[string]any{
			"method": method,
			"id":     id,
			"params": params,
		})
		_, err := fmt.Fprintf(stdin, "%s\n", b)
		return id, err
	}

	if err := initialize(send, lines); err != nil {
		fmt.Fprintf(os.Stderr, "initialize: %v\n", err)
		os.Exit(1)
	}

	threadID, err := createThread(send, lines)
	if err != nil {
		fmt.Fprintf(os.Stderr, "thread/start: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "thread_id: %s\n", threadID)

	var result string
	for i, turn := range turns {
		fmt.Fprintf(os.Stderr, "\n--- turn %d: %q ---\n", i+1, turn)
		result, err = runTurn(send, lines, threadID, turn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "turn %d: %v\n", i+1, err)
			os.Exit(1)
		}
	}

	fmt.Fprintf(os.Stderr, "\n--- validation ---\n")
	// result is accumulated from item/agentMessage/delta notifications,
	// not the full JSON event, so contains is safe here.
	if strings.Contains(result, "12") {
		fmt.Fprintln(os.Stderr, "PASS: result contains 12, context preserved across turns")
	} else {
		fmt.Fprintf(os.Stderr, "FAIL: expected \"12\" in result, got: %s\n", result)
		os.Exit(1)
	}
}

type sendFn func(method string, params any) (int, error)

// readUntilResponse reads messages until the response for the given ID arrives.
// Notifications received in the meantime are printed and discarded.
func readUntilResponse(lines <-chan string, id int) (json.RawMessage, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return nil, fmt.Errorf("stream closed before response to id %d", id)
			}
			var msg rpcMsg
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			pretty, _ := json.MarshalIndent(msg, "", "  ")
			fmt.Println(string(pretty))
			if msg.ID != nil && *msg.ID == id {
				if msg.Error != nil {
					return nil, fmt.Errorf("rpc error: %s", msg.Error)
				}
				return msg.Result, nil
			}
		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for response to id %d", id)
		}
	}
}

// readUntilTurnCompleted reads notifications until turn/completed.
// Accumulates text from item/agentMessage/delta chunks.
func readUntilTurnCompleted(lines <-chan string) (string, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var result strings.Builder
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return "", fmt.Errorf("stream closed before turn/completed")
			}
			var msg rpcMsg
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			pretty, _ := json.MarshalIndent(msg, "", "  ")
			fmt.Println(string(pretty))
			switch msg.Method {
			case "item/agentMessage/delta":
				var p struct {
					Delta string `json:"delta"`
				}
				if err := json.Unmarshal(msg.Params, &p); err == nil {
					result.WriteString(p.Delta)
				}
			case "turn/completed":
				return result.String(), nil
			case "turn/failed":
				return "", fmt.Errorf("turn failed: %s", msg.Params)
			}
		case <-timer.C:
			return "", fmt.Errorf("timeout waiting for turn/completed")
		}
	}
}

func initialize(send sendFn, lines <-chan string) error {
	id, err := send("initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "coderoom",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	})
	if err != nil {
		return err
	}
	_, err = readUntilResponse(lines, id)
	return err
}

func createThread(send sendFn, lines <-chan string) (string, error) {
	cwd, _ := os.Getwd()
	id, err := send("thread/start", map[string]any{"cwd": cwd})
	if err != nil {
		return "", err
	}
	result, err := readUntilResponse(lines, id)
	if err != nil {
		return "", err
	}
	var r struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &r); err != nil {
		return "", fmt.Errorf("parse thread result: %w", err)
	}
	return r.Thread.ID, nil
}

func runTurn(send sendFn, lines <-chan string, threadID, prompt string) (string, error) {
	id, err := send("turn/start", map[string]any{
		"threadId": threadID,
		"input":    []map[string]any{{"type": "text", "text": prompt}},
	})
	if err != nil {
		return "", err
	}
	if _, err := readUntilResponse(lines, id); err != nil {
		return "", err
	}
	return readUntilTurnCompleted(lines)
}
