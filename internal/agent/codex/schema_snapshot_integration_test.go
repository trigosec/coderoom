//go:build integration

package codex_test

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

var updateSchemas = flag.Bool("update-schemas", false, "update Codex JSON schema snapshots in testdata/schemas/")

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
		gotPath := filepath.Join(tmp, "v2", name)
		got, err := os.ReadFile(gotPath)
		if err != nil {
			t.Errorf("schema %s missing from generated output: %v", name, err)
			continue
		}
		got = canonicalJSON(t, got)
		want = canonicalJSON(t, want)
		if !jsonDeepEqual(got, want) {
			if *updateSchemas {
				if err := os.WriteFile(filepath.Join("testdata", "schemas", name), got, 0o644); err != nil {
					t.Fatalf("update snapshot %s: %v", name, err)
				}
				continue
			}
			t.Errorf("schema %s changed; update testdata/schemas/ to accept the new contract", name)
		}
	}
}

func jsonDeepEqual(a, b []byte) bool {
	var av, bv any
	return json.Unmarshal(a, &av) == nil &&
		json.Unmarshal(b, &bv) == nil &&
		reflect.DeepEqual(av, bv)
}

func canonicalJSON(t *testing.T, b []byte) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		// Preserve verbatim so the test fails loudly rather than rewriting
		// something we couldn't parse.
		return b
	}
	// MarshalIndent provides a stable key order for objects (encoding/json
	// sorts map keys) and normalizes insignificant whitespace.
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return b
	}
	return append(out, '\n')
}

func codexPackage() string {
	if v := os.Getenv("CODEX_VERSION_OVERRIDE"); v != "" {
		return "@openai/codex@" + v
	}
	return "@openai/codex"
}
