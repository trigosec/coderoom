# Code Room: Roadmap

This roadmap is written as an executable checklist. Code Room is early-stage;
items may move as we learn from real usage.

Code Room’s north star (see `docs/design/concept.md`): a CLI-native coordination
layer for multiple coding agents, where the human remains the decision authority
and `git diff` is the shared language of change.

## V1: Single agent (current)

Goal: work comfortably with a single agent in the shared room.

### UI: Compose

[X] Very long lines don’t display well (repro: a 3-line wrapped sentence).
[X] Cursor movement:
  [X] Up should move to the first character when already on the first line.
  [X] Down should move to the last character when already on the last line.

### UI: History

[X] Command records are too verbose:
  [X] Show the command (truncate after first 3 lines).
  [X] Avoid displaying command output inline by default.
  [X] Provide a clear affordance for viewing full command + output (e.g. “focus history then `Ctrl+g`”).
[X] Command prefix alignment:
  [X] Commands currently show `$` but skip the `<glyph> <participant>:` line.
  [X] Add the same participant prefix line used by other content, so command
      lines start at the same column where participant content starts.
[X] Scrolling feedback:
  [X] Make it obvious where you are in the scrollback.
  [X] Add header on top of history, indicating how many “page-ups/page-downs” remain.

### UI: Prompts while busy

- [ ] Decide and implement prompt-while-busy behavior: block with a clear
  in-room message, or queue a single prompt. Confirm whether the protocol
  allows buffering a turn without changing semantics.

### Project hygiene (V1 launch)

- README:
  - Clearly explain what exists today (shared room) vs what is planned (private
    channels, policy/sandbox).
  - Provide a short “install → first run” walkthrough and one canonical demo.
- Add `CONTRIBUTING.md`:
  - How to run tests, how to add features safely, and what review quality looks
    like (link `docs/code-review.md`).

### Definition of done (V1)

- Single-agent shared-room workflow is usable without surprises:
  - agent output + reasoning are readable and clearly distinct
  - commands are readable (not spammy) and discoverably inspectable in full
  - scrolling position is obvious
- Prompts while busy behave predictably (blocked with clear feedback or queued).
- Docs are honest:
  - README matches the current product (shared room only)
  - CONTRIBUTING sets contributor expectations

## V2: Multi-agent room (next)

Goal: “named collaborators” in a single shared room feel real and predictable.

### Capabilities

- Invite, remove, and cancel participants across backends.
- Roster and streaming state that scales to concurrent agent activity.
- Roles, capabilities, and initiative are visible (enforcement can be minimal
  at first).
- Action and context items are not silently dropped — file changes, tool calls,
  and searches appear in the room (see `docs/design/pkg-agent-messages.md`):
  - `fileChange` (patch updates)
  - `mcpToolCall`, `dynamicToolCall`
  - (optional/collapsed) `webSearch`, `imageView`, `imageGeneration`

### Implementation

- Minimum viable persistence: event log with a replay/debugging path, aligned
  with `docs/design/architecture.md`.

## V3+: Private channels, approvals, sandbox/policy (later)

Goal: deliver the interaction model in `docs/design/architecture.md`.

- Private agent channels (operational detail stays private; shared room stays
  high-level coordination).
- Approvals UX that is safe and auditable.
- Sandbox controller + policy engine hardening.

## Tracking conventions

If you track this in issues, keep the structure close to the roadmap:

- Labels: `area:ui`, `area:session`, `area:agent`, `area:messages`, `area:docs`
- Types: `type:bug`, `type:enhancement`, `type:design`
- Priority: `prio:P0`, `prio:P1`, `prio:P2`

Prefer small milestones (3–8 issues) with a visible definition of done.
