# Package design: Codex thread keepalive

This document records the keepalive design chosen for Codex-backed
participants.

## Problem

Observed Codex behavior:

- a thread may expire after roughly 30 minutes of inactivity
- when that happens, process state may still exist but the conversational
  thread context is gone
- rebuilding context on a fresh thread can take around 5 minutes

That makes idle expiry too expensive to treat as a normal path.

## Design goals

- keep an idle Codex thread warm before expiry
- preserve context without creating fake user-visible conversation traffic
- keep the policy in session, not in UI
- hide Codex-specific mechanics behind a narrow agent capability
- avoid overengineering until we have evidence that a stronger fallback is
  needed

## Chosen approach

Codex keepalive starts a maintenance notice turn. A read-only
`thread/read` probe was insufficient because it did not preserve the
model-side context lifetime.

The important split is:

- `internal/session` owns when keepalive happens
- `internal/agent/codex` owns how Codex is touched

The UI only reflects state. It does not drive keepalive.

## Agent contract

Agents that support idle preservation expose:

```go
type Keepaliver interface {
    KeepAliveSchedule() time.Duration
    KeepAlive() error
}
```

For Codex:

- `KeepAliveSchedule()` returns `20 * time.Minute`
- `KeepAlive()` starts a minimal `turn/start` constrained to an acknowledgement

`KeepAlive()` is fire-and-forget, matching the shape of `SendNotice()`.
Completion is reported through the normal `Read()` path as an internal
`agent.KeepAlive{}` message.

## Session ownership

Keepalive is a session responsibility.

Why:

- session already owns participant lifecycle and state transitions
- keepalive must coordinate with sendability and request-lane occupancy
- tying the policy to UI would make correctness depend on the interface being
  present and ticking

The session lifecycle is rooted in the top-level `coderoom` context. The
keepalive ticker derives from that session context, so process shutdown cancels
pending keepalive work immediately.

## Scheduling model

Keepalive is scheduled by one session-level ticker.

The ticker interval should be comfortably smaller than the keepalive schedule,
but it does not need second-level precision. A cadence such as `30s` or `1m`
is good enough because Codex expiry is measured in tens of minutes.

`KeepAliveSchedule()` remains the participant's target deadline. The ticker does
not need to fire exactly at that deadline, but it must keep lateness bounded.
The session contract is:

- a participant becomes eligible for keepalive at
  `dueAt = idleSince + KeepAliveSchedule()`
- the sweep may start keepalive after `dueAt`
- the lateness is bounded by one ticker interval

In other words, keepalive should begin within:

```text
[dueAt, dueAt + keepaliveTick]
```

On each tick, session:

1. gets the current time
2. scans participants
3. skips participants that are not `idle`
4. skips agents that do not implement `Keepaliver`
5. skips idle windows younger than that agent's `KeepAliveSchedule()`
6. for each remaining overdue participant:
   - claims the participant under the session lock by atomically transitioning
     `idle -> keepalive`
   - releases the session lock
   - calls `KeepAlive()` out of lock

This makes the scheduler deadline-based rather than waiter-based:

- normal awake runtime is handled by the same sweep
- suspend or hibernate is handled naturally on the next tick after resume
- there is no per-participant keepalive goroutine to cancel or replace

The sweep still uses `participant.Since` as the idle-window identity check, so
it does not need a separate `lastActivityAt` field.

## Why `Since`

We use `participant.Since` as the idle anchor.

That is enough because the question is not "when was there any activity?" in a
generic sense. The question is "has this participant remained in the same idle
period long enough to justify a keepalive?"

Using `Since` avoids adding a separate `lastActivityAt` field.

## Participant state

We add a dedicated participant status: `keepalive`.

Meaning:

- `idle`: available for real work or keepalive scheduling
- `keepalive`: the request lane is occupied by backend maintenance
- `preparing` / `working`: a real turn is in progress

This makes concurrency explicit. A normal send must not overlap a keepalive
request in flight.

## Message flow

Codex keepalive uses the standard message workflow:

1. session calls `agent.KeepAlive()`
2. Codex starts a maintenance notice turn
3. the client suppresses the acknowledgement output
4. `turn/completed` or `turn/failed` becomes `agent.KeepAlive{}`
5. session consumes that message internally and moves the participant from
   `keepalive` back to `idle`

The room/UI does not see a user-visible keepalive record.

The participant may still render as `keepalive` in status-oriented UI such as
the toolbox. "Invisible" here means "no shared-room transcript record is
created for a successful keepalive round-trip."

The keepalive notice uses the same acknowledgement schema as delivery notices, but keeps
its completion semantic distinct: it emits `agent.KeepAlive{}` rather than a
notice-turn stream flush.

## Failure behavior

The first implementation stays intentionally simple.

If `KeepAlive()` fails, or if the keepalive response reports an RPC error:

- Codex emits an internal keepalive completion message
- session returns the participant from `keepalive` to `idle`
- a log event is emitted for observability

We do not add automatic resume or thread rebuild as part of keepalive yet.

Reason:

- if this simple path works, we avoid extra complexity
- if it does not preserve TTL, behavior degrades toward what we already have
  today
- only after we have data should we decide whether a stronger fallback is worth
  it

## Tradeoffs and rejected approaches

### Read-only thread probe

Rejected after implementation experience showed that `thread/read` detects a
thread but does not keep its model-side context warm.

### Ticker-based polling loop

Accepted as the primary scheduling mechanism for keepalive.

Reason:

- the keepalive window is coarse, so exact wake-up precision is not important
- one session ticker is easier to reason about than one goroutine per idle
  participant
- suspend or hibernate no longer needs a special recovery path

Tradeoff:

- keepalive runs on the next sweep after a participant becomes due, not exactly
  at the due timestamp

### Participant-lifetime keepalive context

Rejected because the scheduler no longer uses per-idle waiters. The only
keepalive-owned lifetime is the session ticker itself.
