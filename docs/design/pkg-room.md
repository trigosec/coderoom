# Package design: internal/room

## Scope

The room package owns the chat/room state.

It sits between:

- `internal/session`, which publishes runtime events and owns agent/process
  coordination
- `internal/ui`, which renders rooms and records for the human

The room package is **not** responsible for:

- agent lifecycle
- command execution
- participant state transitions
- policy enforcement

It is responsible for:

- defining rooms
- defining records
- defining room membership
- projecting `session.Event` into chat-visible room state
- maintaining streaming and completed records over time
- buffering room-relevant runtime events off the session observer path
- notifying the UI when room state should be redrawn

---

## Why this package exists

Today the UI assembles semantic chat state directly from `session.Event`.

That is too much responsibility in the rendering layer. The product needs a
stable room/record model that can later support:

- handoff
- copy/reuse actions on prior outputs
- richer room types (shared room, private rooms, etc.)
- expansion/collapse of large records

So the new split is:

- session owns runtime facts
- room owns in-memory room state
- UI owns presentation

---

## Core model

The room package should define two main concepts:

### Room

A room is the in-memory state for one visible collaboration space.

Version 1 likely needs only:

- shared room

Future versions may add:

- private agent rooms
- approval/system-focused rooms

Room is the general data model, not the UI state container.

That means:

- `room.Room` owns participant membership for that room
- `room.Room` owns record membership, ordering, identity, and completion state
- `room.Room` owns the underlying record content and metadata
- `room.Room` does **not** own presentation-only state such as:
  - collapsed vs expanded
  - viewport scroll position
  - selection/focus
  - transient styling state

That presentation state belongs in `internal/ui`.

Participant membership in a room may be:

- all participants in the session, or
- a subset of session participants

The session remains the source of truth for participant runtime state and
transitions. Room maintains only the membership list for a particular room —
which aliases belong to it — not a mirrored snapshot of their status, role,
or approval state.

For V1, membership comes from the same agent-lifecycle events Room already
consumes for system records: `KindAgentStarted` adds an alias,
`KindAgentStopped`/`KindAgentCrashed` remove it. No new event is needed for
the shared room, where membership is simply every participant that has
joined and not since departed. `KindAgentStarting` still produces its own
"starting" system record but does not add membership — an alias is a member
once it has actually joined, not while it's still coming up.

Mutating membership for a future subset/private room — adding or removing
one alias from one room without affecting others — has no documented
mechanism yet. That's out of scope for V1 and needs its own design once
private rooms are introduced; this doc should not be read as already having
solved it.

Room does not project participant or approval state. Both already have a
direct, working path from session to UI today (`session.Roster()` for
participant display, `KindApprovalRequested`/`KindApprovalCleared` consumed
directly by the UI for approval prompts), and neither needs Room's
involvement. Routing this through Room as well would mean inventing new
`session.Event` fields purely to make event-only reconstruction possible
(participant identity such as role/initiative/color is never otherwise
carried on an event) and would duplicate state that's already available
more simply. See "UI integration" for how participant/approval display stays
on its existing path.

### Record

A record is one chat-visible unit within a room.

Examples:

- a user broadcast
- a direct send to an agent
- the routed recipient set for that send
- an agent's completed visible reply
- a system/lifecycle notice
- a handoff

The key point is:

- records are semantic chat units
- records are not raw transport fragments
- records are not UI view models

`room.Record` should contain the canonical content and metadata for the chat
unit. The UI may wrap a `room.Record` with additional per-view state, but that
state should not live in `room.Record` itself.

---

## Input / output boundary

### Input

The room package consumes `session.Event`.

`session.Event` remains the canonical runtime event model. Room does not define
its own peer event stream for the same facts.

In practice, the room package should expose a session-facing projection type
that can consume those events directly. V1 should make `room.Room` a
`session.Observer` for chat/record projection. The UI continues to register
as its own separate `session.Observer` for participant and approval state,
exactly as it does today — see "Session integration".

V1 should prefer the simpler shape:

```go
r := room.New(...)
s := session.New(..., session.WithObserver(r))
```

That keeps the dependency one-way:

- UI calls `session` for effectful actions
- `session` publishes `session.Event`
- `room.Room` observes those events and updates chat-visible state
- `room.Room` emits redraw notifications
- UI listens to room updates and renders snapshots from `room.Room`

The important boundary is:

- session emits runtime facts
- room consumes runtime facts and updates in-memory room state
- room, not UI, owns the async release/buffering off the session observer path
- UI does not re-derive chat semantics from session events

### Output

The room package exposes room state and record updates to consumers such as the
UI.

The UI should render room state, not derive chat semantics from `session.Event`
directly.

The UI also needs a direct path to append user-authored records that do not
originate from agent runtime events.

Examples:

- `/invite ada`
- `/help`
- local validation errors
- startup tips

Those should be added to the room model directly through room-owned APIs rather
than being stored as UI-only history.

This direct insertion path is only for local, non-session records. If a record
represents session/runtime behavior, it should reach room through
`session.Event`, not through a UI shortcut.

---

## Relationship to session events

`session.Event` and room records are not the same thing.

### `session.Event`

- runtime fact
- coordination-oriented
- may be low-level / streaming
- owned by session

### `room.Record`

- chat-visible unit
- projection-oriented
- may accumulate multiple runtime events into one stable message
- owned by room

Example:

- several `KindAgentMessage` streaming events may contribute to one completed
  agent-output record

That accumulation belongs in room, not in UI.

User-authored records are different: they are created intentionally by the UI
and inserted into room directly. They are not reconstructed from
`session.Event`.

---

## Streaming behavior

Room must support in-progress records.

For agent output, a likely model is:

- first streaming output event opens an in-progress record
- subsequent output events update that same record
- flush/finalization closes the record

This preserves the current streaming UX while removing accumulation logic from
the UI.

The room package therefore owns:

- record identity
- open vs completed state
- accumulation of visible output text for one logical record
- accumulation of routing metadata when multiple runtime events contribute to
  one visible record

Routing accumulation needs a correlation key, not arrival order. A
`KindSharedSend` record's routing list is built up as `KindSharedNotice`
events arrive afterward, and the only reliable way to know which record a
given notice belongs to is `session.Event.SendID`. Room should key its
"open" `SharedSend` record by `SendID` while it is still accepting notices for
it, the same way it keys an open streaming record by `agent.StreamID`. This
removes the need to rely on session's internal locking order, which is not
part of the event contract.

---

## Record metadata

Records should carry enough metadata for future actions without forcing the UI
to scrape rendered text.

Likely metadata includes:

- room identity
- record identity
- record kind
- alias / author
- room-visible participant recipients for routed messages, keyed by `SendID`
  while the record is still accepting `KindSharedNotice` events
- source turn ID or stream identity when relevant
- full text payload
- optional neutral preview / summary text when the product needs a canonical
  short form independent of UI rendering
- completion state

This matters for future commands such as handoff, copy, inspect, or summarize.

The canonical record content should live in `room.Record`. Any UI-specific
derived state such as "collapsed" should be maintained separately by the UI and
keyed by room ID + record ID.

---

## Handoff implications

The room package is the natural place to expose the latest completed chat-visible
agent output as a reusable record.

That means `/handoff` should resolve from room-owned completed records rather
than:

- querying the UI viewport/history directly
- reassembling low-level `session.Event` fragments ad hoc in the command path

This is one of the main reasons to introduce room as a package-level concept.

---

## Session integration

Session publishes runtime events.

Room subscribes to those events and updates its model.

Room is a session observer for chat/record projection:

- `room.Room` implements `session.Observer`
- `room.Room` buffers/releases session events off the observer path
- `room.Listener` notifies consumers that room state changed and should be
  redrawn
- UI reads chat/record state from room rather than receiving raw
  `session.Event` for that purpose

Room is not the only observer. The UI keeps its own, separate
`session.Observer` registration for participant and approval state —
`session.Roster()` and direct `KindApprovalRequested`/`KindApprovalCleared`
handling are unchanged by introducing room. `pkg-session.md` already
documents multiple observers as supported; this is that pattern in use, not
an exception to it.

These two paths are independently paced. Room buffers/coalesces before
notifying its `Listener` (see "Concurrency"); the UI's direct registration
does not. That means a participant or approval change can render before, or
after, the chat record that prompted it, by however long Room's coalescing
window is. This is accepted, not accidental: approval prompts and
participant status render in their own UI regions, not inline with chat
text, so strict ordering between the two paths isn't required for
legibility today. If a future feature needs strict ordering across the two
paths, that's a new requirement to solve explicitly then, not something
this split already guarantees.

Session remains the source of truth for runtime coordination state. Room keeps a
projected in-memory model for one room:

- room membership
- record state

This gives the system two distinct observer boundaries:

- runtime observer: session -> room
- redraw observer: room -> UI

Session should not need to know how many records a particular event becomes, or
how the UI chooses to render them.

This keeps the session focused on orchestration.

### Concurrency

`session.Observer.OnEvent` may be called from agent-reader goroutines, so room
must not push Bubble Tea work or other slow listener logic directly on that
path.

The contract should therefore be:

- `room.Room.OnEvent(session.Event)` returns quickly
- room owns any buffering/coalescing needed to free the session observer path
- room emits a lightweight redraw/invalidation notification to listeners
- UI reacts to that notification by reading room state snapshots

V1 should use invalidation-only room updates rather than payload-carrying
record deltas. A room update means "this room changed; re-read its snapshot",
not "here is the changed record." A minimal shape is:

```go
type Update struct {
    RoomID  room.ID
    Version uint64 // optional monotonic revision for stale-update detection
}
```

The UI should treat `Update` as a redraw hint and then call snapshot APIs such
as `Records()` to obtain the actual data.

`internal/ui/queue.go`'s `eventQueue` already solves exactly this
decoupling problem (unbounded buffer between a fast producer goroutine and a
slower consumer pull loop) for the UI's own direct `session.Observer` path.
Room's buffering should reuse or extract that pattern rather than write a
second implementation of the same primitive.

---

## UI integration

For chat/record rendering, the UI should depend on room state rather than on
raw `session.Event`.

That means:

- UI no longer owns record assembly
- UI renders `room.Room` / `room.Record` for chat/record state
- UI continues to render participant and approval state from its own
  direct `session.Observer` registration and `session.Roster()`, unchanged
  from today
- UI may maintain view-local state for a room record, such as collapsed/expanded
- UI-specific concerns remain in UI:
  - viewport
  - styling
  - focus
  - collapsed/expanded interaction

The room package should stay presentation-agnostic.

---

## Routing and rooms

The old `router` abstraction is the wrong level for this system.

Routing decisions belong partly to:

- session command execution and policy
- room projection semantics

They do not justify a separate package whose only purpose is “route messages
across channels.”

The room package provides the visible destination structure; session decides
what to send and to whom.

---

## Design boundary

Session owns:

- commands
- policies
- agent lifecycle
- runtime events

Room owns:

- rooms
- room membership
- records
- projection of runtime events into chat-visible state
- room-local insertion of user-authored records
- buffering/release off the session observer path
- notification of room-state redraws to consumers

UI owns:

- rendering
- interaction
- viewport/focus/selection
- per-record view state such as collapsed/expanded
- its own direct `session.Observer` registration for participant and
  approval state, in parallel with room's observer registration for chat

---

## Initial API direction

The first implementation should make the three concepts explicit:

- `Room`: canonical in-memory room model
- `Record`: canonical chat-visible unit
- `Listener`: outbound redraw/update notification to UI

A plausible V1 shape:

- `type Room struct { ... }`
- `type Record struct { ... }`
- `type Update struct { RoomID ID; Version uint64 }`
- `func (r *Room) OnEvent(e session.Event)`
- `func (r *Room) AppendRecord(rec Record)`
- `func (r *Room) Records() []Record`
- `type Listener interface { OnRoomUpdate(Update) }`

The exact names may change, but the architectural constraint should hold:

- room owns projection and canonical in-memory room state for chat/records
- room is a session observer for chat/record projection, not the only one —
  UI keeps its own separate observer registration for participant/approval
  state
- UI owns display state built on top of room-owned records

---

## Open questions

These are known gaps in this design, deliberately deferred rather than
blocking V1:

- **Whether `session.Event` should move toward one type per `Kind`.**
  `Event` is a flat struct where field meaning depends on `Kind` (`Alias`
  means "addressee" for `KindSharedSend`, "notified listener" for
  `KindSharedNotice`). The missing `SendID` was a symptom of this, now fixed
  with a single additive field. Whether the whole struct should become a
  sealed interface with one concrete type per kind is a separate, much larger
  question — it touches every emission site in `session`, every `switch
  e.Kind` in `room` and `ui`, and all event-based test fixtures. Not a
  prerequisite for shipping room; worth its own design doc only if more gaps
  like the `SendID` one keep showing up. One candidate shape, smaller than
  full per-kind types: split `Event` into two delivery buckets instead of
  one — a room-relevant stream (chat/record kinds) and a global stream
  (approval, participant status) — making the dual-observer split in
  "Session integration" explicit at the type level instead of by
  convention. Doesn't by itself resolve the cross-path ordering question
  raised there; would need its own follow-up either way.

---

## Summary

`internal/room` should become the canonical chat data model of the product.

- `session.Event` stays canonical for runtime facts
- room projects those facts into rooms and records
- UI renders rooms and records

This removes semantic chat assembly from the UI and gives future features such
as handoff a stable non-UI source of truth.
