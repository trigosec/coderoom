# UI design: barrier-batch staging model for multi-participant messaging

Status: implemented (initial)

This document proposes an alternative to "send-now vs queued" semantics: a
barrier-batch staging model.

## Motivation

With multiple participants, mixing real-time delivery (idle participants get messages
immediately) and turn-based delivery (busy participants receive messages later) can
become confusing quickly:

- Did participant A already receive the message while participant B is still working?
- If the user sends multiple follow-ups, did idle participants receive earlier
  versions while busy participants will receive the concatenation?
- How do we reason about coordination when participants are at different "steps"?

Barrier-batch reframes the UX as: "prepare the next turn for a set of participants,
then dispatch it when they are all ready."

## Core idea

Instead of enqueuing messages per participant, the UI maintains a *staged batch* that
represents the next instruction payload for the room.

Dispatch is barrier-based:

- A batch is delivered only when **all** participants in the batch's barrier set are
  idle (ready to accept a new turn), or the user explicitly interrupts them.

This removes "partial delivery" as a default behavior: a batch either has not
been delivered, or it has been delivered to the whole target set.

The barrier gates on **all** participants that are available at the time the batch
is staged, including those receiving a message for awareness only (e.g. via `@alias`
routing). This sequences all interactions and prevents any participant from
receiving the next message while another is still working on the current one. The
result is a consistent shared-history invariant: all participants see the same
sequence of messages in the same order.

### Targets and routing

In the default case, a user-authored message is dispatched to all participants in
the room.

If the message begins with `@alias`, it is still dispatched to all participants,
but it carries an explicit routing intent:

- the addressed participant is the actor (expected to take action)
- other participants are notified for awareness

Barrier-batch treats both actor and notified participants as part of the same
barrier: dispatch waits until *everyone* in the batch is idle (or is explicitly
interrupted).

## Scope decision

- This model applies to user-authored messages only.
- Participant-to-participant messaging is out of scope.

## UX overview

This section defines the proposed UI for the barrier-batch model.

### Staged composer

When the user submits a message while one or more participants are busy, the
composer **transitions to staged mode**:

- the submitted text is rendered greyed-out (read-only)
- keystrokes do not edit the buffer while staged
- an explicit status line appears, e.g.:

  `Message on-hold. Participants busy: ada, turing. Press Esc to edit. Press Ctrl+X to interrupt and send.`

  Aliases are colored per their assigned participant color.

The staged composer replaces the normal composer in the layout — there is no
separate queue pane above it. The composer surface itself is the staging area.

The staged composer returns to normal editing mode when:

- `Esc` is pressed (user wants to edit before sending), or
- `Ctrl+X` is pressed (Interrupt + Send), or
- all participants in the batch's barrier set become idle (automatic dispatch);
  the composer is cleared and ready for new input.

If all participants are already idle when the user hits Enter, dispatch happens
immediately and the composer never enters staged mode — it clears as normal.

### History browsing while staged

While the composer is in staged mode, the user retains full access to history:

- `Ctrl+O` switches focus to the history viewport.
- All history-mode commands (`Ctrl+G`, etc.) remain available.
- The staged status line remains visible regardless of focus.

### Send modes

This model intentionally supports two explicit routing intents while keeping the
same barrier rule (always gate on everyone in the batch's barrier set):

1. **Send all**: no explicit `@alias` prefix; all participants are actors by
   default.
2. **Send one**: message begins with `@alias`; the addressed participant is the
   actor and the rest are notified for awareness.

## Definitions

### "Idle" signal

Barrier-batch requires an explicit "participant is idle" signal from the session /
participant adapter, not a heuristic derived from output streams.

Conceptually:

- idle: the participant has no active turn in progress
- busy: the participant is not currently routable for shared-room delivery

For routing and barrier purposes, `busy` includes:

- `preparing`
- `keepalive`
- `working`
- startup windows such as `starting` / `attached`

`keepalive` is busy even though it is not a user-visible turn. It occupies the
request lane until the backend maintenance round-trip completes.

## State machine (staged batch)

This section defines the minimal states for a staged batch and what triggers
transitions.

### Barrier set and availability snapshot

When a batch is staged, the UI snapshots the set of participants that are
*available* at that moment. This snapshot is the batch's barrier set and dispatch
set.

- Newly invited participants are not retroactively added to an existing staged
  batch.
- Participants that crash or are removed after staging are marked `discarded` and
  excluded from both the barrier and the eventual dispatch.

### Batch states

```
draft ──(user submits; all participants idle)──> dispatching
draft ──(user submits; 1+ participants busy)───> awaiting_barrier
awaiting_barrier ──(all participants idle)────> dispatching
awaiting_barrier ──(user presses Esc)─────────> draft
awaiting_barrier ──(user Interrupt+Send)──────> interrupting
interrupting ──(blocked participants idle)────> dispatching
dispatching ──(dispatch started)──────────────> dispatched
draft/awaiting_barrier ──(user clears)────────> cleared
```

## Behavioral rules

### Text composition

In staged mode there is exactly one staged batch at a time:

- Pressing `Enter` either dispatches immediately (all participants in the batch's
  barrier set are idle) or stages the batch (one or more participants are busy).
- While staged, the user cannot submit additional messages. To change or extend
  the staged payload, the user presses `Esc` to return to draft and modifies the
  text before resubmitting.

### Target set semantics

The set of participants a staged batch considers (barrier + dispatch) is fixed at
the time of staging. The model does not retroactively add newly invited
participants to an existing staged batch.

### Interrupt behavior

Interrupt is always explicit.

- `Ctrl+X` (`Interrupt + Send`) requests cancellation of in-flight turns for the
  batch's blocked participants, then proceeds to dispatch the batch to all
  non-discarded participants in the batch snapshot once they are idle.

## Transcript interaction

Input only appears in the history when the staged text is dispatched.

"Dispatched" means "dispatch attempt started" — participants do not confirm receipt.
The staged text is committed to the transcript/history at dispatch start. Avoid
adding additional history records for per-target delivery churn.

Emit a system record only for disruptive actions (e.g. `[→ ada] interrupt requested`).

## Edge cases

### Crash during dispatching (partial dispatch)

The "barrier" guarantee ends at the start of dispatching: once a dispatch attempt
begins, delivery is best-effort and may partially succeed.

Recovery is user-driven using normal primitives (e.g. re-send a new batch to the
missing participants, or Interrupt + Send to re-synchronize).
