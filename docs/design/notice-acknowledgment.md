# Design: notice acknowledgment filtering

When a `SharedSendCommand` targets one agent, all other agents in the session
receive a listener notice via `TextListeners`. The problem: LLMs treat every
message as a prompt and respond with a full turn. Listener agents should receive
context silently — they should not produce output unless explicitly addressed.

Status: design (not yet implemented).

---

## Goals

- Listener agents receive broadcast context without generating visible output.
- Non-compliant responses (agent ignores the instruction) surface as reasoning
  records rather than main agent output, and do not go unnoticed.
- The filtering logic lives entirely inside the codex adapter. The session and TUI
  layers are unaffected.
- No change to `Read()` semantics: acknowledged turns produce no messages; the
  caller never sees them.

---

## Agent interface

A new method is added to `agent.Agent`:

```go
type Agent interface {
    Start() error
    Send(prompt string) error
    SendNotice(prompt string) error // send context; filter acknowledgment response
    Read() (Message, error)
    Stop() error
}
```

`SendNotice` behaves like `Send` from the caller's perspective: it writes a turn
and returns immediately. The difference is entirely in how the adapter handles the
response.

---

## Session routing

`SharedSendCommand.execute` already sends `TextListeners` to non-addressed agents
via `Send`. That call changes to `SendNotice`:

```go
// before
a.Send(cmd.TextListeners)

// after
a.SendNotice(cmd.TextListeners)
```

No other session-layer changes are required.

---

## Message format

The notice text is wrapped with a CONTEXT UPDATE prefix before sending to the
agent. The prefix instructs the model to return a minimal acknowledgment and
nothing else:

```
[CONTEXT UPDATE — respond only with {"acknowledge":true}]

<original notice text>
```

The expected acknowledgment is the JSON object `{"acknowledge":true}`. This is
chosen because:

- It is valid, parseable JSON.
- It is distinct enough that it is unlikely to appear at the start of a genuine
  prose response.
- Parsing uses `json.Unmarshal` into a typed struct, so spacing variants
  (`{ "acknowledge": true }`) and key ordering are handled correctly.

The acknowledgment shape is always a JSON object. Array-shaped responses are
never valid acks and will never be introduced. This is an invariant the `{`
heuristic (see below) depends on.

No output schema constraint is sent in `turn/start` for now. The prompt
instruction alone is the mechanism; a schema constraint can be added later if
reliability warrants it.

---

## Codex adapter internals

### Turn ID tracking

When `SendNotice` writes a `turn/start` request, it records the request's RPC ID
in a `pendingNoticeReqs` set. The `turn/start` response carries the `turnId`
assigned by Codex. When that response is received in the read loop, the turn ID
is moved from `pendingNoticeReqs` into a `noticeTurnIDs` set. All subsequent
processing for that `turnId` is subject to acknowledgment filtering.

```
SendNotice → writes turn/start with id=N
           → pendingNoticeReqs[N] = true

read loop: sees response {id:N, result:{turnId:"t42"}}
         → delete pendingNoticeReqs[N]
         → noticeTurnIDs["t42"] = true
```

### Buffering and the `{` heuristic

For a turn whose ID is in `noticeTurnIDs`, the read loop inspects deltas before
queuing them:

1. If no buffering is active for this turn and the first non-whitespace character
   of the delta is **not** `{`: the turn is treated as non-compliant immediately.
   All subsequent deltas for this turn are relayed as `MessageReasoning`. No
   further buffering.

2. If the first non-whitespace character **is** `{`: start buffering. Append every
   subsequent delta to the buffer. Do not emit any messages to `Read()` callers.

3. On `turn/completed` for a buffered turn: evaluate the buffer (see below).

4. On `turn/failed` or read error during a buffered or relaying notice turn:
   discard the buffer; emit nothing. The turn is gone.

### Acknowledgment check

On `turn/completed` for a buffered notice turn:

```go
type ackResponse struct {
    Acknowledge bool `json:"acknowledge"`
}
var r ackResponse
if err := json.Unmarshal([]byte(buf), &r); err == nil && r.Acknowledge {
    // compliant — discard silently; do not emit MessageDone
    delete(noticeTurnIDs, turnID)
    return
}
// non-compliant — fall through to relay
```

Any JSON response containing `"acknowledge": true` is treated as compliant and
discarded, regardless of other fields present. `json.Unmarshal` into a struct
silently ignores unknown fields; this is the intended behaviour, not a bug.
No `MessageDone` is emitted; the participant never transitions to `working`.

### Non-compliant relay

If the buffer does not match the acknowledgment format, the accumulated text is
replayed as reasoning:

```
buffered text → emit as MessageReasoning (one message, full body)
turn/completed → emit MessageDone
```

The participant transitions `working → idle` as normal. The output surfaces in
the shared room as a faint `◈ alias (thinking)` reasoning record, not as a main
agent response. This reflects the semantic reality: the agent responded to a
context update it was not asked to act on.

### State cleanup

`noticeTurnIDs` entries are removed when the turn completes (compliant or not) or
when the process stops. Entries do not accumulate across turns.

---

## Sequence diagrams

### Compliant acknowledgment

```
SendNotice("...")  →  turn/start{id:1} → Codex
                   ←  response{id:1, turnId:"t1"}
                   ←  item/agentMessage/delta '{'
                   ←  item/agentMessage/delta '"acknowledge":true}'
                   ←  turn/completed{turnId:"t1"}

Read() callers: nothing emitted
```

### Non-compliant relay

```
SendNotice("...")  →  turn/start{id:1} → Codex
                   ←  response{id:1, turnId:"t1"}
                   ←  item/agentMessage/delta 'I think...'
                   ←  item/agentMessage/delta ' the code looks fine.'
                   ←  turn/completed{turnId:"t1"}

Read() callers see:
    {Kind: MessageReasoning, Text: "I think... the code looks fine."}
    {Kind: MessageDone}
```

---

## Other agent backends

Future backends (Claude Code, Aider, etc.) implement `SendNotice` however suits
their protocol. A simple default wraps the prompt with the CONTEXT UPDATE prefix
and calls `Send` — no filtering, no turn ID tracking. For backends that do not
support structured outputs, the LLM may still respond; that response passes
through `Read()` unfiltered. Filtering is a codex-specific optimisation, not a
contract.

---

## Open questions

- **Schema constraint in `turn/start`**: Codex's `TurnStartParams` may support an
  output schema field. If prompt-only compliance is unreliable in practice, adding
  a schema constraint is the next step.
- **Delta granularity**: the `{` heuristic assumes the first delta contains at
  least one non-whitespace character. If Codex emits a delta with only whitespace
  first (e.g. a leading newline), the heuristic still works correctly — it skips
  whitespace before testing the character. No special case needed.
- **`BroadcastCommand` and initiative**: a broadcast goes to all agents as a real
  `Send`, not `SendNotice`. Agents with `InitiativeManual` should not respond to
  broadcasts not addressed to them — that is a routing concern, not a notice
  concern, and is deferred to the router design.
