// Package linestream provides small utilities for turning a stream of text
// into a stream of lines, and coalescing lines into multi-line batches.
package linestream

import (
	"bufio"
	"io"
	"strings"
	"time"
)

const (
	defaultFlushAfter = 75 * time.Millisecond
	defaultMaxLines   = 80
)

// Config controls line batching behavior.
type Config struct {
	// FlushAfter is the quiet-period after which the current batch is emitted.
	FlushAfter time.Duration
	// MaxLines is the maximum number of lines per batch before forcing a flush.
	MaxLines int
}

// DefaultConfig returns the defaults used for batching process stderr.
func DefaultConfig() Config {
	return Config{
		FlushAfter: defaultFlushAfter,
		MaxLines:   defaultMaxLines,
	}
}

func normalizeConfig(cfg Config) Config {
	if cfg.FlushAfter <= 0 {
		cfg.FlushAfter = defaultFlushAfter
	}
	if cfg.MaxLines <= 0 {
		cfg.MaxLines = defaultMaxLines
	}
	return cfg
}

// Scan returns a channel of lines read from r. The returned channel is closed
// when r reaches EOF.
func Scan(r io.Reader) <-chan string {
	out := make(chan string)
	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			out <- scanner.Text()
		}
		close(out)
	}()
	return out
}

// BatchReader is a convenience that combines Scan + Batch for the common case
// of batching lines directly from an io.Reader using DefaultConfig.
func BatchReader(r io.Reader) <-chan string {
	return Batch(Scan(r), DefaultConfig())
}

// BatchReaderWithConfig is like BatchReader but uses cfg.
func BatchReaderWithConfig(r io.Reader, cfg Config) <-chan string {
	return Batch(Scan(r), cfg)
}

// Batch coalesces lines into multi-line strings according to cfg. The returned
// channel is closed after the input channel closes and any remaining lines are
// flushed.
func Batch(in <-chan string, cfg Config) <-chan string {
	out := make(chan string)
	go func() {
		b := newBatcher(out, in, normalizeConfig(cfg))
		b.run()
		close(out)
	}()
	return out
}

type batcher struct {
	out        chan<- string
	in         <-chan string
	flushAfter time.Duration
	maxLines   int

	// ready holds fully-formed chunks waiting to be delivered to out.
	ready []string
	// incoming accumulates the current chunk as individual lines.
	incoming []string
	// timer fires after a quiet period (no recent lines) to flush the batch.
	timer *time.Timer
}

func newBatcher(out chan<- string, in <-chan string, cfg Config) *batcher {
	return &batcher{
		out:        out,
		in:         in,
		flushAfter: cfg.FlushAfter,
		maxLines:   cfg.MaxLines,
	}
}

func (b *batcher) run() {
	// High-level batching algorithm:
	// - Read lines from `in` into `incoming`
	// - Restart a quiet-period timer on each line
	// - Flush `incoming` into `ready` when:
	//   - the timer fires (no recent lines),
	//   - maxLines is reached, or
	//   - the input closes
	// - Deliver `ready` to `out` in order
	for {
		if b.in == nil && len(b.ready) == 0 {
			return
		}
		out, ready := b.nextReadyChunk()
		timerCh := b.flushTimer()
		select {
		case line, ok := <-b.in:
			if !ok {
				// Disable input receive case; loop will drain any ready chunks
				// through the normal send path and then exit.
				b.flushIncoming()
				b.in = nil
				continue
			}
			b.addIncomingLine(line)
		case <-timerCh:
			b.flushIncoming()
		case out <- ready:
			b.ready = b.ready[1:]
		}
	}
}

func (b *batcher) addIncomingLine(line string) {
	b.incoming = append(b.incoming, line)
	if len(b.incoming) >= b.maxLines {
		b.flushIncoming()
		return
	}
	b.startTimer()
}

func (b *batcher) flushIncoming() {
	if len(b.incoming) == 0 {
		return
	}
	b.ready = append(b.ready, strings.Join(b.incoming, "\n"))
	// Reset length to 0 while keeping capacity for reuse (avoids allocations).
	b.incoming = b.incoming[:0]
	b.stopTimer()
}

func (b *batcher) startTimer() {
	b.stopTimer()
	b.timer = time.NewTimer(b.flushAfter)
}

func (b *batcher) stopTimer() {
	if b.timer == nil {
		return
	}
	if !b.timer.Stop() {
		select {
		case <-b.timer.C:
		default:
		}
	}
	b.timer = nil
}

func (b *batcher) nextReadyChunk() (chan<- string, string) {
	if len(b.ready) == 0 {
		return nil, ""
	}
	return b.out, b.ready[0]
}

func (b *batcher) flushTimer() <-chan time.Time {
	if b.timer == nil {
		return nil
	}
	return b.timer.C
}
