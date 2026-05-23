//go:build integration

package codex_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

var updateSchemas = flag.Bool("update-schemas", false, "update Codex JSON schema snapshots in testdata/schemas/")

// TestSchemaSnapshot compares the mounted Codex JSON schemas against the
// snapshots in testdata/schemas/. Only the schemas in testdata are checked —
// they represent our contract surface. A diff means the Codex protocol changed;
// update testdata/schemas/ to accept the new contract.
func TestSchemaSnapshot(t *testing.T) {
	srcDir := schemaSourceDir()

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
		gotPath := filepath.Join(srcDir, name)
		got, err := os.ReadFile(gotPath)
		if err != nil {
			t.Errorf("schema %s missing from mounted schemas: %v", name, err)
			continue
		}
		got = canonicalJSON(t, got)
		wantCanonical := canonicalJSON(t, want)

		if *updateSchemas {
			// In update mode, always rewrite the snapshot into canonical form so
			// formatting/key-ordering remains stable even when the upstream output
			// is semantically unchanged.
			if err := os.WriteFile(filepath.Join("testdata", "schemas", name), got, 0o644); err != nil {
				t.Fatalf("update snapshot %s: %v", name, err)
			}
			continue
		}

		if !jsonDeepEqual(got, wantCanonical) {
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

func schemaSourceDir() string {
	// This repo mounts the up-to-date Codex schema snapshots at
	// `.mount/codex-json-schema`. Keep this overrideable for contributors with
	// a different mount location.
	if v := os.Getenv("CODEX_JSON_SCHEMA_DIR"); v != "" {
		if filepath.Base(v) == "v2" {
			return v
		}
		return filepath.Join(v, "v2")
	}

	// Resolve relative to this file's location so the test works regardless of
	// the process working directory.
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join(".mount", "codex-json-schema", "v2")
	}
	dir := filepath.Dir(here)
	for {
		cand := filepath.Join(dir, ".mount", "codex-json-schema", "v2")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Join(".mount", "codex-json-schema", "v2")
}
