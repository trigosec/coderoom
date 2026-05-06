//go:build integration

package codex_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
)

// TestSchemaSnapshot regenerates Codex JSON schemas and compares them against
// the snapshots in testdata/schemas/. Only the schemas in testdata are checked —
// they represent our contract surface. A diff means the Codex protocol changed;
// update testdata/schemas/ to accept the new contract.
func TestSchemaSnapshot(t *testing.T) {
	tmp := t.TempDir()
	cmd := exec.Command("npx", codexPackage(), "app-server", "generate-json-schema", "--out", tmp)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("generate-json-schema: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join("testdata", "schemas"))
	if err != nil {
		t.Fatalf("read testdata/schemas: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		want, err := os.ReadFile(filepath.Join("testdata", "schemas", name))
		if err != nil {
			t.Errorf("read testdata %s: %v", name, err)
			continue
		}
		got, err := os.ReadFile(filepath.Join(tmp, "v2", name))
		if err != nil {
			t.Errorf("schema %s missing from generated output: %v", name, err)
			continue
		}
		if !jsonDeepEqual(got, want) {
			t.Errorf("schema %s changed; update testdata/schemas/ to accept the new contract", name)
		}
	}
}

// TestClientSingleTurn verifies basic communication with the Codex app-server.
func TestClientSingleTurn(t *testing.T) {
	cwd, _ := os.Getwd()
	c := codex.New(cwd)
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	result, err := agent.SendAndWait(c, "What is 2 + 2?")
	if err != nil {
		t.Fatalf("SendAndWait: %v", err)
	}
	if !strings.Contains(result, "4") {
		t.Errorf("expected result to contain '4', got: %s", result)
	}
}

// TestClientContextPreservation verifies that context is maintained across
// turns within a single thread.
func TestClientContextPreservation(t *testing.T) {
	cwd, _ := os.Getwd()
	c := codex.New(cwd)
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	if _, err := agent.SendAndWait(c, "What is 2 + 2?"); err != nil {
		t.Fatalf("first turn: %v", err)
	}
	// result is accumulated from item/agentMessage/delta notifications,
	// not the full JSON event, so contains is safe here.
	result, err := agent.SendAndWait(c, "Multiply that result by 3.")
	if err != nil {
		t.Fatalf("second turn: %v", err)
	}
	if !strings.Contains(result, "12") {
		t.Errorf("expected result to contain '12' (context preserved), got: %s", result)
	}
}

func jsonDeepEqual(a, b []byte) bool {
	var av, bv any
	return json.Unmarshal(a, &av) == nil &&
		json.Unmarshal(b, &bv) == nil &&
		reflect.DeepEqual(av, bv)
}

func codexPackage() string {
	if v := os.Getenv("CODEX_VERSION_OVERRIDE"); v != "" {
		return "@openai/codex@" + v
	}
	return "@openai/codex"
}
