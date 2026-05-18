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
├──────── compose ──────────────▲─┤  ← top separator (label + ▲ when scrolled)
│ ❯ input text                    │  ← compose input (grows with content)
├─────────────────────────────▼───┤  ← bottom separator (▼ when scrolled)
│ ◆ ada (10s)  ● bob              │  ← toolbox: participant cells
└─────────────────────────────────┘
```

The viewport occupies all available height minus the compose input, two
separator lines, the toolbox row, and vertical margins. All regions resize on
`tea.WindowSizeMsg`.

The compose input is variable-height (grows as the user types newlines or long
wrapping lines) up to a cap of `min(8, terminal_height/3)`. When the content
exceeds the visible area, `▲` appears on the top separator and/or `▼` on the
bottom separator.

The toolbox sits below the compose area and renders only the participant cells
row. The separator lines framing the compose area are owned by the room
component, not the toolbox.

---

## Bubble Tea model

```go
// Top-level application model.
type Model struct {
    sess     *session.Session
    queue    *eventQueue
    room     room.Model     // history viewport + compose/approval input
    toolbox  toolbox.Model  // participant cells row
    palette  colorPalette
    debug    bool
    cwd      string
    lastSize tea.WindowSizeMsg
}

// room.Model owns the scrollable history and the active input area.
// room.inputModel switches between compose and approval modes.
// history.Model wraps a bubbles/viewport for the output records.
// compose.Model wraps a bubbles/textarea for text entry.
// approval.Model handles approval prompts (option list + keyboard navigation).
```

The room component owns all content rendering and the two separator lines that
frame the compose area. The toolbox is a sibling, not a child, of the room.

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

Streaming state: when a `KindDelta` arrives for an alias not currently streaming,
a new agent-output record is appended to the history and marked as in-progress.
Subsequent deltas for the same alias update that record in place. `KindDone`
closes the record and clears the streaming flag for that alias.

Records are owned by `history.Model`, which wraps a `bubbles/viewport` and
re-renders content on every change. Line wrapping is handled by the viewport
itself; `inlinefmt` applies ANSI-aware styling (colours, hanging indents) per
record kind before the content reaches the viewport.

---

## Command parsing

Input is parsed on submit (Enter):

| Input | Command | Notes |
|---|---|---|
| `/invite <alias>` | `InviteCommand` | |
| `/cancel <alias>` | `CancelCommand` | Soft stop: cancels in-flight work for the agent but keeps it in the room |
| `/remove <alias>` | `RemoveCommand` | Hard stop: removes the agent from the room and stops its process |
| `/who` | — | Renders current roster inline; no session command needed |
| `/help` | — | Renders available commands inline |
| `@<alias> <text>` | `SharedSendCommand` | |
| `<text>` | `BroadcastCommand` | Equivalent to direct send for single-agent sessions |
| `/quit` | `session.Shutdown()` + `tea.Quit` | Best-effort stop all agents before exit |

`/who` and `/help` are handled entirely in the TUI — they append a formatted block to `lines` without touching the session. Validation errors (empty alias, unknown command) are also appended to `lines` as inline messages rather than using a separate error state.

### Command semantics (room model)

- `/invite <alias>` adds the agent to the shared room and starts its process.
- `/cancel <alias>` is a **soft stop**: it attempts to interrupt the agent's
  in-flight work but keeps the agent in the room (joined).
- `/remove <alias>` is a **hard stop**: it removes the agent from the room and
  stops its underlying process.

Open questions (deferred to implementation):
- What "stop" means per backend (true cancel vs best-effort stop/restart).
- How to surface "stop not supported" in the UI without noise.

---

## Dependencies

```
github.com/charmbracelet/bubbletea         # framework
github.com/charmbracelet/bubbles/viewport  # scrollable history output
github.com/charmbracelet/bubbles/textarea  # multi-line compose input
github.com/charmbracelet/lipgloss          # styling
github.com/rivo/uniseg                     # display-width-aware line metrics
```

The session and agent packages are unchanged.

---

## Boundary

The TUI owns:
- Bubble Tea model, update, and view
- Observer channel and `awaitEvent` wiring
- Rendering session events as styled text
- Parsing user input into session commands

The session controller owns everything else. The TUI never reads session internals directly — it only calls `Execute` and receives events through the observer channel.
