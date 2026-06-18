# Design: `/handoff <from> <to>`

## Goal

Provide a simple, explicit way to pass one agent's latest user-visible result
to another agent without:

- broadcasting all agent output to the whole room
- relying on inline reference syntax
- silently inferring agent-to-agent context from plain prose

Version 1 is intentionally small:

- `/handoff <from> <to>` transfers context only
- `@<to> <text>` remains the way to give new instructions

Example:

```text
/handoff tim ada
@ada continue from that and propose the patch
```

---

## Core decision

`/handoff` should not be modeled as “UI history lookup plus a notice send.”

Instead, it should push the design toward the event model that already exists
conceptually in the repo:

- the session owns structured events and appends them to the event log
- the UI renders those events as chat/history records
- handoff resolves its source from event-backed output state, not from the
  viewport's rendered history

This avoids making the UI the source of truth for coordination features.

---

## Why now

Today the product is still a shared-room-only chat:

- the user sees one room
- agents coordinate there
- the human must be able to inspect what signal was transferred

That means `/handoff` needs:

- explicit source and destination
- predictable idleness rules
- strong auditability
- a stable runtime representation that session and UI can both project

Those needs line up naturally with introducing a first-class room/record model
between session runtime notifications and the UI's rendering layer.

---

## Command semantics

Command:

```text
/handoff <from> <to>
```

Meaning:

1. Resolve the latest eligible completed output event for `<from>`.
2. Build a context payload with provenance.
3. Deliver that payload to `<to>` through a context-transfer path.
4. Emit a handoff event that the UI renders in the shared room.

The handoff itself carries no new tasking. It only transfers context.

If the human wants the receiving agent to act on it, they send a follow-up:

```text
@<to> ...
```

---

## Transfer unit

Version 1 should use one exact transfer unit:

- the latest completed user-visible output record from `<from>`'s last
  completed turn

Inclusion:

- completed `agent.Output` only

Exclusion:

- reasoning
- command execution messages
- file change messages
- logs
- partial or still-streaming output
- synthetic notice output

This rule should be defined once and shared by both session behavior and UI
representation.

---

## Event-backed ownership

### Problem

Right now the UI owns the assembly of visible history from streaming session
events, while the session owns routing and lifecycle. That is sufficient for
rendering, but underspecified for commands that need to refer back to prior
agent output.

### Decision

We should start treating the event log / event model as the durable ownership
boundary for “what happened in the room.”

That implies:

- session interacts with the pool of structured events
- UI represents those events as records
- `/handoff` resolves from those structured events, not by querying rendered UI
  history

### Consequence

The design needs a stable event-level notion of:

- completed user-visible output
- source identity
- source turn identity
- payload text

This is the missing runtime unit that both chat rendering and handoff should
share.

---

## Handoff event model

`/handoff` should introduce a distinct event concept rather than overloading
generic shared notices.

At the design level, the handoff event should capture:

- source alias
- destination alias
- source turn ID or source output record ID
- transferred payload text
- preview text for room rendering
- timestamp

The canonical runtime event model should remain in the session package, with
room/record projection layered on top of it.

Possible event taxonomy extension:

- `MessageSent` remains generic user-to-agent room messaging
- new event such as `ContextHandedOff` records explicit agent-to-agent transfer

The exact type names can change later; the important point is that handoff is a
distinct semantic event, not merely an implementation detail of notice sends.

---

## Policy ownership

`/handoff` should be governed by policy, not encoded ad hoc inside the command.

The policy package is the right place to start expressing room-level decisions
such as:

- whether user-to-one-agent sends notify the rest
- which participant-idleness rule applies to handoff

Version 1 should define a handoff policy with explicit modes:

- `all-idle`
- `to-idle`
- `from-and-to-idle`

### Version 1 default

Use:

- `all-idle`

Rationale:

- simplest to reason about in a shared-room product
- matches current synchronized-room expectations better than mutating context
  while some agents are still mid-turn
- easiest to explain and audit

This is stricter than necessary, but safer for the first release.

---

## Recipient state semantics

Version 1 should not mutate an in-flight recipient turn.

Rule:

- if handoff policy is not satisfied, reject the command before sending any
  context

For the default `all-idle` policy, that means:

- if any participant is not idle, `/handoff` is rejected

This is intentionally conservative. A future version may stage handoff behind
the barrier model or allow narrower policies, but version 1 should optimize for
predictability over throughput.

---

## Transport semantics

At the semantic layer, `/handoff` is **context transfer**, not a generic notice.

That suggests a distinct concept such as:

- `SendContext`

Whether that becomes a new `agent.Agent` method immediately is an
implementation decision for later. The design point is:

- `SendNotice` is awareness of another user command
- handoff is explicit user-authorized context transfer from one agent's prior
  output to another agent

Implementation reuse under the hood is acceptable; semantic reuse is not.

---

## Payload shape

The transferred payload sent to `<to>` should preserve provenance and be stable
enough to debug.

A simple version is enough:

```text
[HANDOFF from tim]
[SOURCE TURN <id>]

<full transferred output>
```

Version 1 should not summarize or transform the source output.

---

## Auditability

Auditability is a hard requirement in the current product because the UI is
still a single shared room.

So `/handoff` should create a real history record in the room, not just a thin
marker.

### Shared room rendering

Default room rendering should show a concise handoff record with preview, for
example:

```text
[handoff tim → ada]
<preview of transferred content>
```

The full transferred content should remain inspectable from the history UI
(for example via a focused/detail action such as `ctrl+g`).

### Why this matters

The human needs to be able to answer:

- what was handed off
- from whom
- to whom
- from which source turn / output

Without that, the feature becomes opaque and weakens the “human validates the
signal” workflow.

---

## Failure model

The command should fail before any send when:

- `<from>` does not exist
- `<to>` does not exist
- `<from> == <to>` (version 1 should reject for clarity)
- no eligible completed output exists for `<from>`
- the active handoff policy is not satisfied
- destination is not ready to receive context

Open implementation question for later:

- whether partial delivery is even possible once handoff becomes a distinct
  command path. Version 1 should aim for an atomic “resolve, then send” model.

---

## Why not broadcast all outputs

Forwarding every agent output to every other agent would match a naive chat-room
model, but it has poor properties:

- token growth
- hidden coupling
- less predictable downstream behavior
- weaker human understanding of exactly what each agent received

`/handoff` keeps cross-agent context explicit.

---

## Why not inline syntax first

Inline syntax such as `#tim` may still be useful later, but it is not needed to
unlock the workflow.

Starting with `/handoff` is safer because:

- explicit intent
- stronger validation
- easier auditability
- no parser ambiguity with `@alias` routing

If inline references are added later, they should resolve to the same
event-backed source model rather than creating a second handoff path.

---

## Future extensions

Possible later work:

- configurable handoff policy beyond `all-idle`
- `/handoff <from> <to> --last N`
- `/handoff <from> <to> --all-since-last`
- `/handoff <from> <to> --summary`
- inline references
- record-selection-driven handoff from focused history records
- initiative-aware automatic context passing for autonomous agents

These are out of scope for version 1.

---

## Summary

Version 1 should implement:

- `/handoff <from> <to>`
- source resolution from event-backed completed output state
- transfer unit = latest completed user-visible output from `<from>`
- distinct handoff semantics, even if notice transport is reused internally
- policy-owned idleness rule, defaulting to `all-idle`
- auditable room record with preview and inspectable full payload

This keeps the first handoff feature explicit, predictable, and aligned with a
future event-centered architecture.
