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
    session   *session.Session
    events    <-chan session.Event
    viewport  viewport.Model
    input     textinput.Model
    lines     []string        // accumulated rendered lines
    streaming map[string]bool // agents currently mid-turn
    cwd       string
    ready     bool            // true after first WindowSizeMsg
}
```

`lines` is the source of truth for viewport content. Every session event appends to it; the viewport is re-set from `strings.Join(lines, "\n")` after each change.

---

## Observer → Bubble Tea bridge

The session observer runs on agent reader goroutines. Bubble Tea's `Update` runs on its own goroutine. The bridge is a buffered channel and a long-running `tea.Cmd`:

```go
// sessionEventMsg wraps a session.Event as a Bubble Tea message.
type sessionEventMsg session.Event

// awaitEvent returns a Cmd that blocks until the next session event arrives.
func awaitEvent(ch <-chan session.Event) tea.Cmd {
    return func() tea.Msg {
        return sessionEventMsg(<-ch)
    }
}
```

The observer writes to the channel non-blocking (same pattern as `testObserver` in session tests). `Init` returns `awaitEvent(ch)` to start the loop. Each time `Update` handles a `sessionEventMsg` it returns `awaitEvent(ch)` again to re-arm.

The channel is created by the TUI and passed to `session.WithObserver` at construction, decoupling the session package from the Bubble Tea import.

---

## Event rendering

| Event kind | Rendered as |
|---|---|
| `KindAgentStarted` | `[ada joined]` |
| `KindAgentStopped` | `[ada left]` |
| `KindAgentCrashed` | `[ada crashed]` |
| `KindBroadcast` | `[all] <text>` |
| `KindSharedSend` | `[→ ada] <text>` |
| `KindSharedNotice` | `[notice → ada]` |
| `KindDelta` | streamed inline: `ada> <fragment>` on first delta, subsequent fragments appended to the same line |
| `KindDone` | closes the current streaming line |

Streaming state: when a `KindDelta` arrives for an alias not currently streaming, a new line starting with `alias> ` is appended to `lines` and marked as in-progress. Subsequent deltas for the same alias append to the last element of `lines` in place. `KindDone` closes the line and clears the streaming flag.

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
