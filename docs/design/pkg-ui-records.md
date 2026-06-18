# UI design: record-based rendering

## Motivation

The current model accumulates output as a flat `[]string` of lines. This produces a wall of undifferentiated text that is hard to scan. The record model replaces it with structured units that carry authorship, visual identity, and routing metadata, making conversations readable at a glance.

---

## Record

A record is the atomic unit of viewport content. Each session event maps to one record, with one exception: consecutive `KindDelta` events for the same alias coalesce into a single `recordKindAgentOutput` record until the matching `KindDone` closes it.

```go
type recordKind int

const (
    recordKindUserInput  recordKind = iota // what the user typed
    recordKindAgentOutput                  // streaming response from an agent
    recordKindSystem                       // lifecycle and routing notices
    recordKindLog                          // agent diagnostic line (stderr)
)

type record struct {
    kind    recordKind
    alias   string // agent alias; empty for user input and system records
    body    string // accumulated content; grows during streaming
    routing []string // aliases shown in the footer (broadcast / direct send)
}
```

`body` is the canonical, unstyled source of truth. Wrapping and styling are applied at render time, not stored.

---

## Visual layout

Records are separated by a blank line to give the output room to breathe.

### User input

```
  > /invite ada

```

The `>` prefix is styled (bold or dimmed) to distinguish it from agent output. The line is echoed immediately on Enter, before any system response.

### Agent output

```
  ●  ada:
  This is the agent's response, which may span
  multiple lines as deltas stream in.

```

The header line is `● <alias>:` — the filled circle and alias text are both rendered in the agent's assigned colour, followed by a plain colon. The body renders below the header, left-aligned. No per-line prefix — the header provides authorship for the whole record.

### System record

```
  [ada joined]

```

Single line, dimmed. Used for lifecycle events (`joined`, `left`, `crashed`). Listener routing notices are not rendered as separate system records; the sender record footer already shows the full routing list.

**Exception**: `/help` output is a multi-line system record for readability. Splitting it into individual records would insert blank lines between each help line, making the output hard to scan.

### Log record

```
  ▸ npm warn deprecated package

```

Single line, greyed out (lipgloss colour 240). Identical to current rendering; just modelled as a record.

### Routing footer

When the user sends a broadcast or direct message, the input record shows who was addressed. Each alias in the footer is rendered in that agent's assigned colour:

```
  > hello everyone
  → ada    → turing

```

For a direct send (`@ada do the thing`) only the addressed agent appears:

```
  > @ada do the thing
  → ada

```

The `→` arrow is plain; only the alias text is coloured. The footer is part of the `recordKindUserInput` record and is populated from `record.routing` at render time. It is omitted when `routing` is empty (e.g. `/invite`, `/who`, `/help`).

---

## Agent colour

Each agent is assigned a colour from a fixed palette when it joins. The colour is stored in `participant.Participant.Color` (a hex colour string, e.g. `"#4ade80"`) and used wherever the agent's alias appears (header bullet, routing footer).

**Palette** (hex, spread across the hue wheel for contrast on dark backgrounds):

| Slot | Colour | Hex |
|------|--------|-----|
| 0 | Green  | `#4ade80` |
| 1 | Blue   | `#60a5fa` |
| 2 | Amber  | `#fbbf24` |
| 3 | Pink   | `#f472b6` |
| 4 | Purple | `#c084fc` |
| 5 | Orange | `#fb923c` |
| 6 | Cyan   | `#22d3ee` |
| 7 | Lime   | `#a3e635` |

Assignment is round-robin. Colour is released when the agent leaves but not reused within the same session, to avoid confusion between past and present participants.

**Departed agents**: when an agent stops or crashes, all of its historical records (agent output headers, routing footer aliases) are immediately re-rendered in `ColorDeparted` (`#6b7280`, muted grey). This repaint persists across terminal resizes. The effect signals that the output belongs to a past participant without losing the authorship structure of the record.

---

## Streaming

Streaming output maps naturally onto the agent output record. When the first `KindDelta` for an alias arrives, a new `recordKindAgentOutput` is opened and appended to the record slice. Subsequent deltas for the same alias extend `record.body` in place. `KindDone` closes the record (marks it as no longer streaming).

`streaming map[string]int` maps alias → index in the records slice, the same role it plays today for `lines`.

The streaming record's body grows on every delta. The viewport is re-rendered from the records slice on each update; only the open record's rendered form changes.

---

## Wrapping

Wrapping moves from `syncViewport` into the record renderer. Each record renders to a `string` that fits `viewport.Width`. The agent output record uses a hanging indent equivalent (content left-aligned under the header, not prefixed). The flat `wrappedLines []string` cache is replaced by a per-record rendered cache: `renderedRecords []string` where each entry is the fully rendered, wrapped, styled text for one record. On resize, all records are re-rendered.

---

## Data model changes

| Current | Replaced by |
|---|---|
| `lines []string` | `records []record` |
| `wrappedLines []string` | `renderedRecords []string` |
| `linePrefixes []string` | per-record render function (no stored prefix) |
| `streaming map[string]int` | same, index into `records` |

`appendLine` is replaced by `appendRecord(r record)`. `handleDelta` opens or extends a `recordKindAgentOutput` record. `handleEnter` creates a `recordKindUserInput` record before dispatching the action.

---

## Open / deferred

- Acknowledgement footer (who confirmed receipt): deferred; requires session-level tracking not yet designed.
- Multi-line user input (paste): out of scope for Phase 1.
