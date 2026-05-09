# Package design: selection and copy while scrolling

This document explores how Code Room could support convenient text selection and
copy while also supporting viewport scrolling. It describes two options and
their tradeoffs. This is not part of the v1 implementation.

## Background: why this is hard in terminals

Terminal-native text selection and in-app mouse wheel scrolling compete for the
same underlying mechanism: xterm mouse reporting. When a TUI enables mouse
reporting to receive wheel/drag events, many terminal emulators stop performing
their normal click-drag selection (often requiring Shift+drag as an override).

As a result, a TUI typically chooses one of:

- Do not capture mouse: terminal selection works; scrolling must be keyboard
  (or via terminal scrollback when not in alt-screen).
- Capture mouse: wheel scrolling works; terminal-native selection is impaired,
  so selection/copy must be implemented in-app to keep UX smooth.

## Current v1 stance

Keep v1 “as-is”:

- Prioritize terminal-native selection/copy (no mouse reporting required).
- Provide keyboard viewport scrolling (`PgUp`/`PgDn` half-page) and “follow mode”
  (only auto-scroll when already at bottom).

This avoids forcing Shift+mouse selection for common copy/paste workflows.

## Option A: full in-app selection + copy (mouse captured)

Goal: allow mouse wheel scrolling and “normal-feeling” selection while the app
captures mouse events.

### UX sketch

- Mouse wheel: scroll viewport.
- Mouse drag: selects text in the viewport (in-app selection, not terminal
  selection).
- `Esc`: clear selection / exit selection mode.
- `Ctrl+C` (or `y` in copy-mode): copy selected text to clipboard.
- Optional: keyboard-only selection mode as a fallback for terminals where mouse
  drag is undesirable.

### Technical requirements

- Mouse reporting enabled at program level.
- A selection model over the viewport content:
  - selection start/end positions in “content coordinates”
  - mapping between screen cells and underlying content positions
- Rendering selection highlight:
  - apply style over the selected range (inverse/colored)
  - handle wrapped lines and ANSI-styled content
- Copy backend:
  - Prefer OSC52 clipboard (works over SSH in many terminals).
  - Fallback: write selection to a temp file and print a hint, or provide an
    internal “clipboard buffer” that the user pastes manually.

### Complexity and risks

- Mapping and selection over wrapped ANSI text is non-trivial.
- Clipboard behavior varies across terminals; OSC52 can be disabled.
- Requires careful UX design to avoid conflicts with scrolling and clicking.

### When to choose

Choose this if “mouse wheel scrolling + drag-to-select inside the app” is a
core product expectation (Claude Code / IDE-like feel) and worth the
implementation complexity.

## Option B: pager-style “copy mode” (tmux/less alternative)

Goal: keep terminal-native selection by default, and offer a focused navigation
and copy flow without building full mouse selection.

Two variants are common:

### B1) External pager/export

- Command/key: e.g. `/export` or `Ctrl+P` to open a pager.
- Implementation: write the transcript to a temp file and launch `less -R` (or
  user-configured pager) on it; return to the TUI on exit.

Pros:
- Very simple to implement and extremely familiar.
- Gives search (`/`), easy navigation, and stable selection/copy.

Cons:
- Context switch out of the TUI.
- Requires pager availability and careful ANSI handling (`-R`).

### B2) In-app copy mode (tmux-like)

- Key: enter a “copy mode” that freezes the viewport and enables cursor-based
  navigation and selection using keyboard.
- Selection is purely in-app; copy uses OSC52 or a fallback.

Pros:
- No external process; works in alt-screen.
- Keyboard-driven, consistent, scriptable.

Cons:
- Still requires selection mapping/highlighting, but less mouse complexity.
- UX needs careful design (mode indicator, exit rules).

### When to choose

Choose this if:
- You want strong copy/search/navigation ergonomics without mouse reporting, or
- You want an incremental path: start with external pager (B1), then evolve to
  in-app copy mode (B2) if needed.

## Recommendation for later exploration

Start with Option B1 (external pager/export). It delivers the biggest UX win for
copy/search/navigation with minimal risk. If users strongly want mouse wheel
scrolling + drag selection inside the UI, revisit Option A with a dedicated
copy/selection design.

