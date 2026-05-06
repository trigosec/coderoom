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

// StopCommand stops and removes an agent from the session.
type StopCommand struct {
    Alias string
}

// BroadcastCommand sends a message to the shared room and to all agents.
// All agents receive the broadcast; initiative governs whether they may
// take action without being explicitly addressed.
type BroadcastCommand struct {
    Text string
}

// SendCommand sends a message directly to one agent's private channel.
type SendCommand struct {
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
    KindAgentStarted Kind = "agent.started"
    KindAgentStopped Kind = "agent.stopped"
    KindAgentCrashed Kind = "agent.crashed"
    KindBroadcast    Kind = "message.broadcast" // shared room message
    KindDelta        Kind = "message.delta"     // streaming text fragment
    KindDone         Kind = "message.done"      // turn complete
)

type Event struct {
    Kind  Kind
    Alias string // participant alias the event relates to
    Text  string // message text or delta fragment
}
```

The relationship between session events and the persistent event log (`internal/event`) is deferred — the two will be connected when session state persistence is implemented.

---

## Agent lifecycle

`InviteCommand` calls `registry.Add` then `agent.Start`. On success, it emits `KindAgentStarted` and launches a reader goroutine for that agent.

The reader goroutine loops on `agent.Read()`, translating `agent.Event` values into session `Event` values and calling `observer.OnEvent`. When `Read()` returns an error, the goroutine checks whether shutdown was requested (via a per-agent context cancellation) to emit `KindAgentStopped` vs `KindAgentCrashed`, then exits.

`StopCommand` signals the reader goroutine to stop, calls `agent.Stop`, then calls `registry.Remove`.

---

## Message routing

| Command | Routing |
|---|---|
| `BroadcastCommand` | Emits `KindBroadcast`; sends text to all agents regardless of initiative |
| `SendCommand` | Sends text to the named agent via `agent.Send`; response events carry the alias |

Shared room visibility is a property of the event kind, not a separate mechanism. The TUI renders `KindBroadcast` events to all views and `KindDelta`/`KindDone` to the relevant agent's private view.

---

## Concurrency model

- One goroutine per agent (the reader loop) — spawned on `InviteCommand`, exits on agent death or `StopCommand`.
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
