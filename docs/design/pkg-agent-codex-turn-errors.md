# Package design: agent turn error visibility

This document records the current step for turn-scoped backend/API failures in
`internal/agent`, starting with the Codex app-server.

Scope:

- turn/API failures that occur after `Start()` succeeded
- how much of that failure should be surfaced to the user now

Out of scope:

- redesigning the `agent.Agent` error model
- startup failures
- process/transport failures
- restart policy

---

## Problem

Turn-scoped backend/API failures can contain useful detail, but that detail is
currently easy to lose.

For the current product stage, the immediate need is modest:

- users should be able to see what went wrong
- we should preserve real error examples before designing a richer taxonomy
- we should avoid a larger control-flow redesign before we understand the error
  shapes better

---

## Codex evidence

The Codex app-server documents an errors flow here:

- <https://developers.openai.com/codex/app-server#errors>

A typical turn failure looks like:

1. an `error` notification is emitted during the turn
2. the turn later completes with `turn/completed` and `status: "failed"`

Important implication:

- a failed turn is not necessarily represented as `turn/failed`
- the useful diagnostic detail may appear first in the `error` notification

Example shape:

```json
{
  "method": "error",
  "params": {
    "error": {
      "message": "...",
      "codexErrorInfo": "...",
      "additionalDetails": null
    },
    "threadId": "...",
    "turnId": "...",
    "willRetry": false
  }
}
```

followed by:

```json
{
  "method": "turn/completed",
  "params": {
    "threadId": "...",
    "turn": {
      "id": "...",
      "status": "failed",
      "error": {
        "message": "...",
        "codexErrorInfo": "...",
        "additionalDetails": null
      }
    }
  }
}
```

---

## Decision

For now, turn/API failures should be surfaced to the user as log events when
the backend provides useful detail.

This is a visibility-first step only.

What stays the same:

- startup failures continue to be handled by `Start() error`
- process/transport failures continue to be handled by `Read()` errors
- existing session lifecycle behavior remains the source of truth for
  participant state

What changes:

- adapter-level turn/API failure details should be preserved and emitted as log
  events when available

Rationale:

- users get immediate visual feedback about failures
- we can collect real examples before freezing a richer error model
- the user can still recover pragmatically by removing and re-inviting an agent
  if it becomes stuck or unusable

---

## Non-goals

This step does not:

- define a typed `TurnError`
- require session to distinguish all turn-failure subtypes
- define room rendering beyond existing log-event behavior
- decide whether failed turns should become a richer semantic outcome later

---

## Follow-up

Before changing the agent error surface, gather more examples of:

- Codex `error` notifications
- `turn/completed` failures
- cases where the process remains reusable after a failed turn
- cases where the user still needs to remove and re-invite the agent

After that, we can decide whether a typed turn-error model is justified.
