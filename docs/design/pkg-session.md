# Package design: internal/session

## Scope

The Session Controller is the central orchestrator of a Code Room session. It receives structured commands, dispatches to agents, and forwards agent output to the appropriate channel.

It is the layer that owns goroutines. The agent package is synchronous; the session controller spawns one reader goroutine per agent to stream output without blocking.

It is **not** responsible for parsing raw user input or rendering output — those belong to the TUI layer.

---

## Input model

The session controller exposes a single entry point:

```go
func (s *Session) Execute(cmd Command) error
```

`Command` is a sealed interface — only types within the `session` package can implement it. Dispatch is via an unexported method; no type switch required:

```go
type Command interface {
    execute(s *Session) error
}
```

The TUI parses raw user input into one of the concrete command types before calling `Execute`:

```go
// InviteCommand adds an agent to the session and starts it.
type InviteCommand struct {
    Backend    string
    Alias      string
    Role       participant.Role
    Initiative participant.Initiative
}

// CancelCommand interrupts in-flight work for an agent but keeps it in the
// registry/room (the agent remains "joined").
//
// Semantics:
// - Best-effort: not all agent backends support true cancellation.
// - Does not remove history/records; it only affects ongoing execution.
type CancelCommand struct {
    Alias string
}

// RemoveCommand stops and removes an agent from the session (hard stop).
type RemoveCommand struct {
    Alias string
}

// BroadcastCommand sends a message to the shared room and to all agents.
// A broadcast is explicitly addressed to all agents (it is a real `Send` to each).
type BroadcastCommand struct {
    Text string
}

// SharedSendCommand sends a message to one agent in the shared room.
// TextDirect is sent to the addressed agent; TextListeners is sent to all
// other agents. The caller supplies both texts — the session controller does
// not format messages. A shared room event is emitted so the TUI displays
// it to everyone.
type SharedSendCommand struct {
    Alias         string
    TextDirect    string
    TextListeners string
}

// PrivateSendCommand sends a message directly to one agent's private channel.
// Nothing is emitted to the shared room and no other agents are notified.
// Used for approval flows and reasoning that should not pollute the shared room.
type PrivateSendCommand struct {
    Alias string
    Text  string
}
```

Each command type carries only the fields it needs. Adding a new command requires implementing `execute` — the compiler enforces it.

---

## Output model

The session controller notifies observers of session events. Observers are registered at construction time via `WithObserver`, following the same pattern as `ProtocolObserver` in the codex package:

```go
type Observer interface {
    OnEvent(e Event)
}
```

Implementations must be fast; avoid operations that can block for non-trivial time. A blocking observer will stall all agent reader goroutines. If the TUI needs to process events on its own goroutine, it puts the event on an internal queue inside its `OnEvent` implementation — the session controller is not responsible for that decoupling.

Multiple observers are supported (e.g. TUI + event logger).

`Event` is defined in the session package:

```go
type Kind string

const (
    KindAgentStarting Kind = "agent.starting"   // agent process is being started
    KindAgentStarted  Kind = "agent.started"    // agent is ready to receive messages
    KindAgentStopped  Kind = "agent.stopped"    // agent was cleanly removed
    KindAgentCrashed  Kind = "agent.crashed"    // agent exited unexpectedly
    KindAgentLog      Kind = "agent.log"        // diagnostic line from the agent (e.g. stderr); always forwarded, rendering is the observer's choice
    KindAgentMessage  Kind = "agent.message"    // typed agent output; see Msg field
    KindBroadcast     Kind = "message.broadcast" // message sent to all agents
    KindSharedSend    Kind = "message.shared"   // instruction to one agent, visible to all
    KindSharedNotice  Kind = "message.notice"   // context notice forwarded to a listener
)

type Event struct {
    Kind  Kind
    Alias string          // participant alias the event relates to
    Text  string          // for KindBroadcast, KindSharedSend, KindSharedNotice, KindAgentLog
    Msg   *agent.Message  // for KindAgentMessage; nil for all other kinds
}
```

`KindAgentMessage` carries the full `agent.Message` value without translation. Observers type-switch on `event.Msg.Content` to handle specific content types (`Output`, `Reasoning`, `Command`, `FileChangeSet`, etc.). See [`pkg-agent-messages.md`](pkg-agent-messages.md) for the message model.

`KindAgentLog` is kept as a dedicated kind with `Text` set directly. This lets observers handle diagnostic lines without inspecting message content.

The relationship between session events and the persistent event log (`internal/event`) is deferred — the two will be connected when session state persistence is implemented.

---

## Agent lifecycle

`InviteCommand` calls `registry.Add` then `agent.Start`. On success, it emits `KindAgentStarted` and launches a reader goroutine for that agent.

The reader goroutine loops on `agent.Read()`, forwarding each message to observers as a `KindAgentMessage` event (or `KindAgentLog` for `Log` content). The session also inspects messages for participant state management — it does not accumulate or translate content:

- First `Output` or `Reasoning` fragment (`ModeStream`) → `MarkWorking`
- Turn-level `ModeFlush` (`codex:turn:<turnId>`) → `MarkIdle`

When `Read()` returns an error, the goroutine checks whether shutdown was requested (via a per-agent stop channel) to emit `KindAgentStopped` vs `KindAgentCrashed`, then exits.

`RemoveCommand` removes the participant from the registry, cancels the reader goroutine's context (so it will emit `KindAgentStopped` rather than `KindAgentCrashed` when it exits), then calls `agent.Stop`.

`CancelCommand` looks up the participant and rejects it if the agent is still starting or has crashed. It calls `agent.Interrupt()`, which is best-effort — the call returns nil for all no-op cases (no active turn, or the backend does not support cancellation). The agent remains in the registry and its reader goroutine continues running.

---

## Message routing

| Command | Routing |
|---|---|
| `BroadcastCommand` | Emits `KindBroadcast`; sends text to all agents regardless of initiative |
| `SharedSendCommand` | Sends `TextDirect` to addressed agent; sends `TextListeners` to all other agents; emits one `KindSharedSend` event (addressed agent) and one `KindSharedNotice` event per notified listener |
| `PrivateSendCommand` | Sends text to the addressed agent only; no shared room event; no other agents notified |

Shared room visibility is a property of the event kind. The TUI renders `KindBroadcast` events to all views and `KindAgentMessage` events to the relevant agent's view (shared or private, depending on routing policy).

---

## Concurrency model

- One goroutine per agent (the reader loop) — spawned on `InviteCommand`, exits on agent death or `RemoveCommand`.
- `Execute` runs on the caller's goroutine (the TUI's input loop).
- Reader goroutines call `observer.OnEvent` directly; the observer must not block.
- The registry and session state are accessed from both `Execute` and reader goroutines. A mutex protects shared access.

---

## Design boundary

The session controller owns:
- Command execution and dispatch
- Agent lifecycle (start, stop, crash detection)
- Message routing to agents
- Emitting session events

The TUI owns:
- Parsing raw user input into commands
- Subscribing to events and rendering them

The router package will own:
- The routing rules as the system grows more complex (multi-agent coordination, initiative-driven dispatch)
