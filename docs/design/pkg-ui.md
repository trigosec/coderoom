# Package design: internal/ui

## Scope

The TUI is the user-facing layer for Phase 1: a single-agent terminal interface that resembles the Codex / Claude Code experience. It owns:

- Rendering session events to a scrollable output area
- Accepting user input and parsing it into session commands
- Bridging the session observer (called from agent goroutines) into the Bubble Tea update loop

It is **not** responsible for session logic, message routing, or agent lifecycle — those remain in `internal/session`.

---

## Layout

```
┌─────────────────────────────────┐
│                                 │
│   scrollable output (viewport)  │
│                                 │
├─────────────────────────────────┤
│ > _                             │  ← text input
├─────────────────────────────────┤
│ ada (builder) · /invite /stop … │  ← toolbox (collapsed, Phase 2+)
└─────────────────────────────────┘
```

The viewport occupies all available height minus the input row, the toolbox row, and separators. All three regions resize on `tea.WindowSizeMsg`; the viewport height calculation uses a variable chrome height so adding or expanding the toolbox later is a one-line change.

The toolbox sits below the input to keep the output→input flow uninterrupted. In Phase 1 it is hidden. From Phase 2 it collapses to a single hint line (agent roster, available commands) and expands upward when needed.

---

## Bubble Tea model

```go
type Model struct {
    sess      *session.Session
    queue     *eventQueue
    viewport  viewport.Model
    input     textinput.Model
    lines     []string        // accumulated rendered lines
    streaming map[string]int  // alias → index in lines for agents mid-turn
    agents    []string        // active aliases for /who
    cwd       string
    ready     bool            // true after first WindowSizeMsg
}
```

`lines` is the source of truth for viewport content. Every session event appends to it; the viewport is re-set from `strings.Join(lines, "\n")` after each change.

---

## Observer → Bubble Tea bridge

The session observer runs on agent reader goroutines. Bubble Tea's `Update` runs on its own goroutine. The bridge is an `eventQueue` — a named type that owns all concurrency for this boundary — and a long-running `tea.Cmd`:

```go
// sessionEventMsg wraps a session.Event as a Bubble Tea message.
type sessionEventMsg session.Event

// awaitEvent returns a Cmd that blocks until the next event is available.
// It receives queue.out — the output channel of eventQueue.
func awaitEvent(ch <-chan session.Event) tea.Cmd {
    return func() tea.Msg {
        e, ok := <-ch
        if !ok {
            return nil
        }
        return sessionEventMsg(e)
    }
}

// Usage in Init and Update:
//   return awaitEvent(m.queue)
```

`eventQueue` owns an unbuffered input channel, an unbounded internal buffer (a plain slice), an output channel, and a pump goroutine that bridges them. `Push` on the input side completes quickly because the pump is always ready to receive. The consumer reads from the output side without ever blocking the producer.

```
session.Observer.OnEvent → eventQueue.Push → [pump goroutine / []Event] → out → awaitEvent → Bubble Tea
```

No fixed-size buffers. No dropped events. If the UI falls behind, the internal slice grows; backpressure propagates naturally through the pump rather than through silent data loss.

`channelObserver` is a thin wrapper that implements `session.Observer` by delegating to `eventQueue.Push`. `Init` returns `awaitEvent(queue.out)` to start the loop. Each time `Update` handles a `sessionEventMsg` it returns `awaitEvent(queue.out)` again to re-arm.

---

## Event rendering

| Event kind | Rendered as |
|---|---|
| `KindAgentStarted` | `[ada joined]` |
| `KindAgentStopped` | `[ada left]` |
| `KindAgentCrashed` | `[ada crashed]` |
| `KindAgentLog`     | `▸ <line>` in grey (lipgloss); de-emphasised diagnostic output; does not participate in streaming state; appended as a standalone line like any other event |
| `KindBroadcast` | `[all] <text>` |
| `KindSharedSend` | `[→ ada] <text>` |
| `KindSharedNotice` | `[notice → ada]` |
| `KindDelta` | streamed inline: `ada> <fragment>` on first delta, subsequent fragments appended to the same line |
| `KindDone` | closes the current streaming line |

Streaming state: when a `KindDelta` arrives for an alias not currently streaming, a new line starting with `alias> ` is appended to `lines` and marked as in-progress. Subsequent deltas for the same alias append to the last element of `lines` in place. `KindDone` closes the line and clears the streaming flag.

Long lines are wrapped before being passed to the viewport. `syncViewport` applies `wrapLine` to each entry in `lines` before joining and calling `SetContent`. `wrapLine` uses `charmbracelet/x/ansi.Wrap` (already an indirect dependency) so wrapping is ANSI-aware. Streaming lines (`alias> …`) use a hanging indent computed from `ansi.StringWidth(prefix)` so continuation text aligns with the first content column. The wrapped output is cached in `wrappedLines` (parallel to `lines`) so only the changed line is re-wrapped on each delta; all lines are re-wrapped on resize.

---

## Command parsing

Input is parsed on submit (Enter):

| Input | Command | Notes |
|---|---|---|
| `/invite <alias>` | `InviteCommand` | |
| `/stop <alias>` | `StopCommand` | |
| `/who` | — | Renders current roster inline; no session command needed |
| `/help` | — | Renders available commands inline |
| `@<alias> <text>` | `SharedSendCommand` | |
| `<text>` | `BroadcastCommand` | Equivalent to direct send for single-agent sessions |
| `/quit` | `session.Shutdown()` + `tea.Quit` | Best-effort stop all agents before exit |

`/who` and `/help` are handled entirely in the TUI — they append a formatted block to `lines` without touching the session. Validation errors (empty alias, unknown command) are also appended to `lines` as inline messages rather than using a separate error state.

---

## Dependencies

```
github.com/charmbracelet/bubbletea       # framework
github.com/charmbracelet/bubbles/viewport  # scrollable output
github.com/charmbracelet/bubbles/textinput # input field
github.com/charmbracelet/lipgloss        # styling
```

No other new dependencies. The session and agent packages are unchanged.

---

## Boundary

The TUI owns:
- Bubble Tea model, update, and view
- Observer channel and `awaitEvent` wiring
- Rendering session events as styled text
- Parsing user input into session commands

The session controller owns everything else. The TUI never reads session internals directly — it only calls `Execute` and receives events through the observer channel.
