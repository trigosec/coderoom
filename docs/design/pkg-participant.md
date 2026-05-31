# Package design: internal/participant

## Scope

`internal/participant` defines the states associated with an agentic session: a named
collaborator with identity (`alias`, `role`, `initiative`) plus turn-lifecycle
state (`status`, tracked streams, turn anchor).

It is not:

- a transport adapter to an external CLI
- the orchestrator that decides when to send work
- a UI model

Those responsibilities belong to `agent`, `session`, and `ui` respectively.

The package exists to centralize participant invariants so they are enforced in
one place instead of being reimplemented ad hoc by the session controller.

---

## State model

Current participant statuses:

- `idle`: no turn is in flight
- `starting`: the agent process is starting and cannot receive work yet
- `preparing`: the session has committed to a new turn, but the participant has
  not entered `working` yet
- `working`: a turn is in flight and turn-scoped stream fragments may arrive
- `crashed`: the agent process exited unexpectedly

The normal lifecycle is:

```text
starting -> idle
idle -> preparing -> working -> idle
* -> crashed
```

`preparing` exists to close the race between "the session decided to send" and
"the participant is visibly in-flight". Once a participant is in `preparing`,
other callers can no longer observe it as available for another direct send.

---

## Invariants

The participant package enforces these runtime rules:

- a participant cannot start a new turn while already `preparing` or `working`
- a participant cannot receive work while `starting` or `crashed`
- a participant cannot become `idle` while its turn anchor is still open
- a stream flush for an untracked stream is invalid
- turn-scoped streams may only be tracked while the participant is
  `preparing` or `working`

This is why the participant is not just a bag of fields. It owns the legality
of state transitions; the session owns when to attempt them.

---

## Transition API

The intended lifecycle is:

1. `BeginStartup`
2. `CompleteStartup`
3. `PrepareForWork`
4. `BeginWorking`
5. `TrackStream` / `CloseStream`
6. `BecomeIdle`

Exceptional paths:

- `AbortWork` rolls back a `preparing` or `working` participant to `idle`
- `Crash` clears turn state and moves the participant to `crashed`

### Why `PrepareForWork` exists

`PrepareForWork` is the race-closure step.

Without it, the caller would need to:

1. observe `idle`
2. call the agent
3. later mark the participant `working`

That leaves a window where another caller can still observe the participant as
idle even though work has already been committed. `PrepareForWork` closes that
window synchronously before the transport call happens.

### Why `TrackStream` accepts `preparing`

The participant allows stream tracking in both `preparing` and `working`.

This is deliberate. A backend may emit stream activity very quickly relative to
the point where the session finishes the state transition into `working`.
Accepting `TrackStream` during `preparing` means those early fragments are still
legal and do not force the session to choose between a race and an invariant
violation.

### Why `BecomeIdle` is stricter than "no visible streams left"

`BecomeIdle` is gated by the participant's turn anchor, not just by whether the
currently observed auxiliary streams have all flushed.

Auxiliary streams such as output, reasoning, command execution, and file-change
messages may open and close in phases during one turn. The participant only
becomes idle when the caller has already closed the anchor stream and then asks
for the final `working -> idle` transition.

That separation is intentional:

- `CloseStream` answers whether the close should end the turn
- `BecomeIdle` validates that ending the turn is now legal

---

## Anchor ownership

The participant stores a single `anchor` field plus the broader
`OpenStreams` set.

- `anchor` means "this stream authoritatively defines turn lifetime"
- `OpenStreams` means "these turn-scoped streams are currently open"

The participant package does not define where anchors come from. That is an
agent-level concern. It only enforces the rule that a participant must not
become idle while its anchor remains open.

---

## Design boundary

`internal/participant` owns:

- participant statuses
- legal state transitions
- tracked-open-stream bookkeeping
- anchor-gated idle validation

It does not own:

- deciding which message to send
- choosing direct send vs notice send
- constructing stream IDs
- translating backend protocol into messages

For those concerns see:

- [`pkg-session.md`](pkg-session.md)
- [`pkg-agent-codex.md`](pkg-agent-codex.md)
- [`pkg-agent-messages.md`](pkg-agent-messages.md)
