//go:build integration

package codex_test

import (
	"flag"
	"os"
	"testing"

	"github.com/trigosec/coderoom/internal/agent/codex"
)

var logDir = flag.String("logdir", "", "directory to write Codex wire logs into (if empty, uses the test temp dir)")

func wireObserverForTest(t *testing.T) codex.ProtocolObserver {
	t.Helper()
	dir := *logDir
	if dir == "" {
		dir = t.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("wirelog-dir mkdir: %v", err)
	}
	f, err := os.CreateTemp(dir, "codex-wire-*.log")
	if err != nil {
		t.Fatalf("wire log temp: %v", err)
	}
	t.Logf("codex wire log: %s", f.Name())
	t.Cleanup(func() { _ = f.Close() })
	return codex.NewLogObserver(f, "codex")
}
