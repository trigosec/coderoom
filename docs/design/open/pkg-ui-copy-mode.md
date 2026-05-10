# Package design: copy mode (read-only output focus) (open)

This document proposes introducing explicit UI modes to simplify scrolling and
copying output in the shared-room TUI, without relying on terminal scrollback or
mouse reporting.

The core idea mirrors established terminal workflows (tmux/less “copy mode”):

- Default mode: compose messages in the input.
- Copy mode: temporarily focus the output area, navigate it, select text, and
  copy; then return to compose mode.

This is a design proposal for later iteration; v1 can remain “compose-first”.

## Goals

- Keep the default experience simple and chat-like.
- Provide a discoverable way to scroll and copy long output.
- Avoid mouse reporting to preserve normal terminal mouse behavior where
  possible.
- Avoid complex “terminal-native transcript” tricks (clear/redraw, scrollback
  duplication).

## Non-goals (for now)

- Mouse-drag selection inside the app.
- Perfect parity with terminal-native selection semantics (double click word,
  etc.).

## Modes

### Compose mode (default)

Primary focus is the input composer.

Expected behavior:

- Input editing works normally (multi-line textarea; arrows move within input).
- `Enter` submits; `Alt+Enter` inserts newline.
- Output viewport remains visible above the input.
- Viewport scrolling via keyboard remains available without stealing arrow keys
  from the input:
  - `PgUp` / `PgDn` scroll half-page.
- Follow behavior: only auto-scroll on new output if the viewport was already at
  bottom (users can scroll up and remain there).

### Copy mode (read-only output focus)

Copy mode focuses the output viewport and makes the input read-only/inactive.

Entry:

- Proposed binding: `Ctrl+S` (placeholder; must not conflict with other core
  bindings).

Exit:

- `Esc` returns to compose mode.

Behavior in copy mode:

- Output viewport consumes navigation keys:
  - `Up` / `Down` scroll line-wise.
  - `PgUp` / `PgDn` scroll half-page.
  - `Home` / `End` jump to top/bottom of the output (optional if supported by
    the viewport).
- Input is visually de-emphasized and does not accept edits (no accidental
  typing).

## Selection and copy

Copy mode is only valuable if the user can copy text without relying on terminal
selection.

### Minimal viable selection (keyboard-driven, nano-style)

Instead of relying on modifier-heavy shortcuts (e.g. Shift+arrows), use an
explicit “mark” model similar to `nano`:

- `Ctrl+6`: start/stop selection (“mark”) at the current cursor position.
- Movement keys adjust the cursor while marked to expand/shrink the selection.
- `Alt+6`: copy selection.
- `Esc`: exit copy mode (and clear any active selection).

Rationale:

- Works reliably across terminals (no dependency on Shift+arrow sequences).
- Easy to teach (“mark, move, copy”) and familiar to users of `nano`.
- Avoids mouse-driven selection complexity while remaining accessible to users
  who are not vi/emacs specialists.

### Copy backend

The implementation must copy text somewhere useful. Options:

1. OSC52 clipboard copy (preferred for terminal TUIs; works over SSH in many
   terminals).
2. Platform integration (`pbcopy`, `wl-copy`, `xclip`) where available.
3. Fallback: write selection to a temp file and print its path (lowest common
   denominator).

Exact selection-to-text mapping must be defined (see open questions).

## UI affordances

- In compose mode, a subtle hint can advertise copy mode:
  - e.g. `Ctrl+S copy-mode · PgUp/PgDn scroll · Ctrl+G editor`
  - Placement: toolbox/status row (Phase 2+) to avoid cluttering the composer.
- In copy mode, a clear indicator should be shown (status line):
  - e.g. `[copy mode]  Esc: back  v: select  y: copy`

## Tradeoffs

Pros:

- Simplifies the “scroll + copy” story without terminal scrollback hacks.
- Avoids mouse reporting and its selection conflicts.
- Keeps the UI in one consistent rendering model (viewport + composer).

Cons:

- Introduces an explicit mode (extra concept to learn).
- Requires in-app selection mapping and a copy backend to be truly useful.

## Open questions

1. Binding choice: which key chord enters copy mode (must be unclaimed and
   ergonomic)?
2. Selection semantics: select by screen cells vs by underlying content (wrapped
   lines and ANSI styling make this tricky).
3. Scope: does copy mode include only the viewport content, or also record
   headers/metadata?
4. Copy destination: should OSC52 be the default, with platform commands as
   fallback, or vice-versa?
