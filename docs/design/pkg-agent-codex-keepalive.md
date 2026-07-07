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

We implement option 2 from the earlier exploration: a read-only keepalive via
Codex `thread/read`.

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
- `KeepAlive()` sends `thread/read {threadId, includeTurns:false}`

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

The session lifecycle is rooted in the top-level `coderoom` context. Idle
waiters derive from that session context, so process shutdown cancels any
pending keepalive timers immediately.

## Scheduling model

We do not use a polling loop.

Instead, keepalive is scheduled per idle window:

1. participant transitions to `idle`
2. session starts one waiter goroutine for that participant
3. the waiter sleeps for the agent's `KeepAliveSchedule()`
4. on wake-up, it checks that:
   - the participant is still `idle`
   - the participant `Since` timestamp is unchanged
5. if both checks still hold, session transitions the participant to
   `keepalive` and calls `KeepAlive()`

If the participant leaves `idle` before the timer fires, the idle-window
context is cancelled and the waiter exits.

This keeps the mechanism tied to actual idle spans rather than continuous
background polling.

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
2. Codex sends `thread/read`
3. Codex client later receives the bare RPC response on stdout
4. the client translates that response to `agent.KeepAlive{}`
5. session consumes that message internally and moves the participant from
   `keepalive` back to `idle`

The room/UI does not see a user-visible keepalive record.

For now, post-start `thread/read` is the only supported bare thread-shaped RPC
response in the Codex adapter. That is why the response classifier can treat a
thread-shaped bare response as keepalive completion without adding correlation
machinery yet.

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

## Rejected approaches

### Synthetic notice turn

Rejected as the primary design because it creates fake conversation traffic,
consumes turn machinery, and muddies history for what should be invisible
transport maintenance.

### Polling loop

Rejected because the session already knows exactly when a participant enters and
leaves `idle`. A per-idle-window waiter is simpler and more precise than a
global periodic sweep.

### Participant-lifetime keepalive context

Rejected because the only useful lifetime for a keepalive waiter is the current
idle span. Creating a fresh context on `idle` and cancelling it on `idle -> *`
keeps the number of live goroutines bounded to actual idle participants.

## Open question

The remaining product question is empirical:

- does `thread/read` only detect expiry, or does it also refresh Codex's idle
  TTL?

The implementation is intentionally structured so we can answer that question
without changing the rest of the app contract.
