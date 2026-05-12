package linestream

import (
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBatchReader_coalescesIntoMultiLineChunk(t *testing.T) {
	r, w := io.Pipe()
	defer func() { _ = r.Close() }()

	chunks := BatchReader(r)

	_, _ = io.WriteString(w, "line1\nline2\nline3\n")
	_ = w.Close()

	chunk, ok := <-chunks
	if !ok {
		t.Fatalf("expected at least one chunk")
	}
	if !strings.Contains(chunk, "line1") || !strings.Contains(chunk, "line2") || !strings.Contains(chunk, "line3") {
		t.Fatalf("expected chunk to contain all lines, got %q", chunk)
	}
	if strings.Count(chunk, "\n") < 2 {
		t.Fatalf("expected multi-line chunk, got %q", chunk)
	}

	// Ensure the channel closes after emitting the (single) chunk.
	select {
	case extra, ok := <-chunks:
		if ok {
			t.Fatalf("expected channel to close, got extra chunk %q", extra)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected chunks channel to close")
	}
}

func TestBatch_flushesAfterQuietPeriod(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FlushAfter = 10 * time.Millisecond

	lines := make(chan string)
	chunks := Batch(lines, cfg)

	lines <- "line1"
	var got []string

	// Wait for quiet-period flush rather than sleeping for a fixed duration.
	select {
	case c := <-chunks:
		got = append(got, c)
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for quiet-period flush")
	}

	lines <- "line2"
	close(lines)

	for c := range chunks {
		got = append(got, c)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks due to quiet-period flush, got %d: %#v", len(got), got)
	}
	if got[0] != "line1" {
		t.Fatalf("expected first chunk to be line1, got %q", got[0])
	}
	if got[1] != "line2" {
		t.Fatalf("expected second chunk to be line2, got %q", got[1])
	}
}

func TestBatch_flushesWhenMaxLinesReached(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FlushAfter = 10 * time.Second // disable timer in practice
	cfg.MaxLines = 3

	lines := make(chan string)
	chunks := Batch(lines, cfg)

	go func() {
		for i := 1; i <= 7; i++ {
			lines <- "l" + strconv.Itoa(i)
		}
		close(lines)
	}()

	var got []string
	for c := range chunks {
		got = append(got, c)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 chunks (3+3+1), got %d: %#v", len(got), got)
	}
	if got[0] != "l1\nl2\nl3" {
		t.Fatalf("unexpected chunk0: %q", got[0])
	}
	if got[1] != "l4\nl5\nl6" {
		t.Fatalf("unexpected chunk1: %q", got[1])
	}
	if got[2] != "l7" {
		t.Fatalf("unexpected chunk2: %q", got[2])
	}
}
