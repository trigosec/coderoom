//go:build integration

package codex_test

import (
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/trigosec/coderoom/internal/agent"
	"github.com/trigosec/coderoom/internal/agent/codex"
)

var logDir = flag.String("logdir", "", "directory to write Codex wire logs into (if empty, uses the test temp dir)")

const testTimeout = 20 * time.Second

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

func startClient(t *testing.T, c agent.Agent) {
	t.Helper()
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
}

// readResponse drains Read() until the turn-end flush and returns the
// accumulated output text. anchorID is the stream ID returned by Send(); when
// non-empty, its ModeFlush is the authoritative turn-end signal. When empty,
// falls back to the heuristic: all observed output streams have flushed.
func readResponse(t *testing.T, c *codex.Client, anchorID agent.StreamID, timeout time.Duration) string {
	t.Helper()
	var sb strings.Builder
	open := make(map[agent.StreamID]struct{})
	seenOutput := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			msg, err := c.Read()
			if err != nil {
				return
			}
			if out, ok := msg.Content.(agent.Output); ok {
				switch msg.Mode {
				case agent.ModeStream:
					seenOutput = true
					open[msg.StreamID] = struct{}{}
					sb.WriteString(out.Text)
				case agent.ModeFlush:
					if anchorID != "" && msg.StreamID == anchorID {
						return
					}
					// SendNotice emits a synthetic turn-end flush on a dedicated
					// stream. Ignore it here so callers waiting for a visible
					// response do not terminate early with an empty string.
					if msg.StreamID == agent.StreamID("codex:notice-turn") {
						continue
					}
					delete(open, msg.StreamID)
					if anchorID == "" && seenOutput && len(open) == 0 {
						return
					}
				}
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("timed out after %s waiting for response", timeout)
	}
	return sb.String()
}
