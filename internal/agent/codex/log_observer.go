package codex

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// LogObserver implements ProtocolObserver by writing every raw JSON-RPC line
// to w with a timestamp, agent alias, and direction arrow. It is safe for
// concurrent use so multiple agents can share a single writer (e.g. one file).
type LogObserver struct {
	mu    sync.Mutex
	w     io.Writer
	alias string
}

// NewLogObserver returns a LogObserver that tags each line with alias and
// writes to w.
func NewLogObserver(w io.Writer, alias string) *LogObserver {
	return &LogObserver{w: w, alias: alias}
}

// OnSend implements ProtocolObserver for outbound messages.
func (o *LogObserver) OnSend(msg string) {
	o.write("→", msg)
}

// OnReceive implements ProtocolObserver for inbound messages.
func (o *LogObserver) OnReceive(msg string) {
	o.write("←", msg)
}

func (o *LogObserver) write(dir, msg string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, _ = fmt.Fprintf(o.w, "%s %s %s %s\n",
		time.Now().UTC().Format(time.RFC3339Nano), o.alias, dir, msg)
}
