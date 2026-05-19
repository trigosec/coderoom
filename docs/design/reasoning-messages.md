# Design: reasoning messages

Reasoning messages carry an agent's internal thinking trace — the chain-of-thought
that precedes a response. This document describes how they are represented in the
session event model, how they affect participant status, and how they are rendered
in the history.

Status: design (not yet implemented).

---

## Goals

- Display reasoning inline so the user can follow an agent's thought process.
- Keep reasoning visually distinct from both agent output and system records.
- Not introduce a new participant status: `working` covers both reasoning and
  responding.
- Design for eventual migration to private rooms without reworking the rendering
  layer.

---

## Session events

One new event kind is added to the session package:

```go
KindReasoningDelta Kind = "message.reasoning" // streaming reasoning fragment
```

`KindDone` (`"message.done"`) closes reasoning streams as well as output streams.
No separate `KindReasoningDone` is introduced — the protocol treats "end of turn"
as a single signal regardless of what was streamed during it.

`KindDone` closes the **current open record** (reasoning and/or output) for the
given alias. If new deltas arrive after `KindDone`, the UI opens a **new record**
and continues streaming into it.

```go
// Event (unchanged structure):
type Event struct {
    Kind  Kind
    Alias string
    Text  string
}
```

A `KindReasoningDelta` event carries the same fields as `KindDelta`. The `Alias`
identifies which participant is reasoning.

---

## Participant status

`KindReasoningDelta` transitions the participant to `working`, exactly as
`KindDelta` does. `KindDone` returns the participant to `idle`.

The toolbox shows no distinction between "working because of a turn" and "working
because of reasoning". The alternating `⏹`/`◆` glyph covers both.

### Interleaved and concurrent streams

Reasoning and output deltas may arrive in any order:

- Sequential (most common): all reasoning deltas arrive, then all output deltas.
  The participant stays `working` throughout; `KindDone` ends both.
- Interleaved: reasoning and output fragments alternate. The participant stays
  `working` as long as either stream is active.
- Concurrent (two agents from separate CLIs): each agent drives its own status
  independently.

Because `KindDone` closes the current open record(s) and deltas can reopen new
records, interleaving does not require special-case protocol handling: the
history appends to an existing record when a slot is open, otherwise it opens a
new record.

---

## History: record model

A new record kind is added:

```go
RecordKindReasoning // streaming reasoning trace from an agent
```

`KindReasoningDelta` events open and extend a `RecordKindReasoning` record in the
same way that `KindDelta` events open and extend a `RecordKindAgentOutput` record.
`KindDone` seals both.

### Streaming slots

The history maintains two independent streaming maps:

| Map | Key | Value |
|---|---|---|
| `streaming` (existing) | alias | index of open `RecordKindAgentOutput` |
| `reasoningStreaming` (new) | alias | index of open `RecordKindReasoning` |

Both maps are keyed by alias. Both are cleared when `KindDone` fires for that
alias. This supports concurrent reasoning and output records for the same agent
without conflict.

If a delta arrives and there is no open slot for that alias, the history creates
a new record of the appropriate kind, appends it, and records its index in the
map. If a delta arrives after `KindDone`, this rule opens a fresh record.

---

## Visual rendering

Reasoning records are rendered similarly to system records, with two differences:

1. **Color**: the participant's assigned colour, rendered faint/dimmed (lipgloss
   `Faint(true)`). System records use the default terminal colour; this is the
   sole visual differentiator.
2. **Prefix**: a glyph marking the record as thinking, e.g. `◈` or a `[thinking]`
   label. The exact glyph is an implementation choice; the design constraint is
   that it is distinct from the agent output header (`● alias:`).

Streaming behaviour mirrors agent output: the body grows as deltas arrive; the
record is re-rendered from source on every delta.

Example (while streaming):

```
  ◈ ada (thinking)
  I need to check what files exist before deciding on an approach...
```

When the agent is departed (stopped or crashed), reasoning records are repainted
in `ColorDeparted` (`#6b7280`) like agent output records.

---

## Room routing

**Phase 1 (this implementation)**: reasoning records are appended to the shared
room, alongside agent output and system records.

**Phase 2 (future, private rooms)**: `KindReasoningDelta` will route to the
agent's private room instead of the shared room. The rendering layer does not need
to change — the record kind and visual format remain identical; only the
destination room changes.

---

## Open questions

- **Exact glyph/prefix**: `◈`
- **Concurrent reasoning + output for the same agent**: the two-slot model handles
  it, but UX for interleaved records (a reasoning record, then an output record
  opening mid-thinking) is untested. We accept this for Phase 1.
