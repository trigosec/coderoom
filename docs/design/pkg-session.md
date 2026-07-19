# Package design: internal/session

## Scope

The Session Controller is the central orchestrator of a Code Room session. It receives structured commands, dispatches to agents, and forwards agent output to the appropriate channel.

It is the layer that owns goroutines. The agent package is synchronous; the session controller spawns one reader goroutine per agent to stream output without blocking.

It is **not** responsible for parsing raw user input or rendering output — those belong to the TUI layer.

State ownership model:

- `agent` is a synchronous transport adapter to the external CLI
- `participant` is the stateful runtime entity
- `session` is the sole mutator/coordinator of participant state
- `ui` projects session/participant state and should not interact with agents directly

Participant invariants and state-machine rules live in
[`pkg-participant.md`](pkg-participant.md). The session does not duplicate those
rules; it coordinates when to invoke them.

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
    Alias string
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

Invite-time participant configuration is resolved inside `session`, not in the
TUI. Session may consult repo-local config using the invited alias, derive the
participant's runtime role, and pass the synthesized startup prompt into the
agent factory so the backend can apply it during startup.

---

## Output model

The session controller notifies observers of session events. Observers are registered at construction time via `WithObserver`, following the same pattern as `ProtocolObserver` in the codex package:

```go
type Observer interface {
    OnEvent(e Event)
}
```

Implementations must be fast; avoid operations that can block for non-trivial time. A blocking observer will stall all agent reader goroutines. If an observer needs to process events on its own goroutine, it puts the event on an internal queue inside its `OnEvent` implementation — the session controller is not responsible for that decoupling.

Multiple observers are supported (e.g. TUI + room + event logger). Per [`pkg-room.md`](pkg-room.md), `room.Room` registers as an observer for chat/record projection only. The TUI continues to register directly as its own `session.Observer` for participant and approval state, exactly as it does today — room does not replace that registration, it adds a second one alongside it.

`session.Event` is the canonical runtime event model for Code Room.

- session publishes `session.Event`
- room consumes `session.Event` and projects rooms + records
- UI renders room state rather than assembling chat semantics directly from
  `session.Event`
- future persistence / replay should derive from `session.Event`

We do **not** want a second peer event model owned by another package. If a
persisted event-log schema is needed later, it should be defined as a
projection of `session.Event`, not as a competing source of truth. See
[`pkg-room.md`](pkg-room.md) for the room/record projection layer that sits
between session events and the UI.

`Event` is defined in the session package as a sealed interface implemented by
concrete event structs:

```go
type Event interface {
    sessionEvent()
}

type AgentStarting struct{ Alias string }
type AgentStarted struct{ Alias string }
type AgentStopped struct{ Alias string }
type AgentCrashed struct{ Alias string }

type AgentLog struct {
    Alias string
    Text  string
}

type AgentMessage struct {
    Alias string
    Msg   agent.Message
}

type ParticipantStatusChanged struct {
    Alias string
    From  participant.Status
    To    participant.Status
    Since time.Time
}

type Broadcast struct{ Text string }

type SharedSend struct {
    Alias string
    Text  string
}

type SharedNotice struct {
    Alias string
    Text  string
}

type ContextHandoff struct {
    FromAlias string
    ToAlias   string
    Text      string
    Preview   string

    SourceRecordIndex int
    BarrierAliases    []string
    IdleAliases       []string
    BusyAliases       []string
    RejectionReason   string
}

type ApprovalRequested struct {
    Alias string
    ID    int64
    Req   agent.ApprovalRequest
}

type ApprovalCleared struct {
    Alias string
    ID    int64
}
```

Observers branch on concrete event type, then on message content when handling
`AgentMessage`. `AgentMessage` carries the full `agent.Message` value without
translation. Consumers type-switch on `event.Msg.Content` to handle specific
content types (`Output`, `Reasoning`, `Command`, `FileChangeSet`, etc.). See
[`pkg-agent-messages.md`](pkg-agent-messages.md) for the message model.

`AgentLog` remains a dedicated event type with `Text` set directly. This lets
observers handle diagnostic lines without inspecting message content.

`ParticipantStatusChanged` is emitted for every `participant.Status`
transition the session drives, including the idle transition after a turn
ends. `From`, `To`, and `Since` are sufficient for an observer that only needs
to track status; full participant identity (role, initiative, color) is read
from `session.Roster()`, not reconstructed from events.

`ApprovalRequested` carries the queue-managed approval `ID`, the participant
`Alias`, and the `agent.ApprovalRequest` payload. `ApprovalCleared` carries the
same `ID` plus the cleared alias so consumers can dismiss the active prompt
without re-reading session state.

---

## Agent lifecycle

`InviteCommand` calls `registry.Add` then `agent.Start`. On success, it emits
`AgentStarted` and launches a reader goroutine for that agent.

Agent process lifecycle is rooted in the session lifecycle context. The
session derives one child context per invited agent and passes that child into
the backend adapter via the configured factory. This gives teardown a strict
hierarchy: session shutdown cancels all agent contexts, while removing or
failing one agent cancels only that agent's subtree.

The reader goroutine loops on `agent.Read()`, forwarding each message to
observers as an `AgentMessage` event (or `AgentLog` for `Log` content). The
session also inspects messages for participant state management — it does not
accumulate or translate content. Open-stream tracking lives on the participant
itself, and the session is the sole mutator of that runtime state. The session
drives participant transitions; the participant validates whether they are
legal:

- Successful `Send` / `SendNotice` calls first commit the participant to the
  turn (`PrepareForWork`), then transition it to `working` once the adapter
  returns the turn anchor
- First `Output`, `Reasoning`, `Command`, or `FileChangeSet` fragment (`ModeStream`) → open a tracked stream for that participant
- Matching `ModeFlush` for one of those streams → close the tracked stream
- Anchor flush for the participant's active turn → `MarkIdle`, which emits
  `ParticipantStatusChanged` (`To: participant.StatusIdle`)

Notice turns are the special case: `SendNotice` may be fully silent, so the
session primes a synthetic `codex:notice-turn` stream on send and closes it when
the adapter emits the matching flush.

When `Read()` returns an error, the goroutine checks whether shutdown was
requested (via a per-agent stop channel) to emit `AgentStopped` vs
`AgentCrashed`, then exits.

`RemoveCommand` removes the participant from the registry, cancels the reader
goroutine's context (so it will emit `AgentStopped` rather than `AgentCrashed`
when it exits), then calls `agent.Stop`.

`CancelCommand` looks up the participant and rejects it if the agent is still starting or has crashed. It calls `agent.Interrupt()`, which is best-effort — the call returns nil for all no-op cases (no active turn, or the backend does not support cancellation). The agent remains in the registry and its reader goroutine continues running.

---

## Message routing

| Command | Routing |
|---|---|
| `BroadcastCommand` | Emits `Broadcast`; sends text to all agents regardless of initiative |
| `SharedSendCommand` | Sends `TextDirect` to addressed agent; sends `TextListeners` to all other agents; emits one `SharedSend` event (addressed agent) and one `SharedNotice` event per notified listener |
| `PrivateSendCommand` | Sends text to the addressed agent only; no shared room event; no other agents notified |

Shared room visibility is a property of the event kind, but the session does
not own the final chat projection. It emits runtime events; the room package
decides how those events become rooms and records for the UI.

---

## Concurrency model

- One goroutine per agent (the reader loop) — spawned on `InviteCommand`, exits on agent death or `RemoveCommand`.
- `Execute` runs on the caller's goroutine (the TUI's input loop).
- Reader goroutines call `observer.OnEvent` directly; the observer must not block.
- The registry and session state are accessed from both `Execute` and reader goroutines. A mutex protects shared access.
- Keepalive candidate selection happens under the session lock, but adapter
  calls such as `KeepAlive()` happen after unlocking. Session first claims the
  participant (`idle -> keepalive`) under lock, then touches the backend out of
  lock so one slow keepalive request cannot stall unrelated reads or sends.

---

## Design boundary

The session controller owns:
- Command execution and dispatch
- Agent lifecycle (start, stop, crash detection)
- Message routing to agents
- Mutation of participant runtime state
- Emitting session events

The room package owns:
- Projection of `session.Event` into chat-visible rooms and records
- Record accumulation / finalization for streaming agent output
- The canonical room data model consumed by the UI

The TUI owns:
- Parsing raw user input into commands
- Rendering room state
- Reading participant/session snapshots
- Forwarding commands to session / room as appropriate

The TUI does not talk to agents directly and should not assemble chat semantics
from raw session events.
