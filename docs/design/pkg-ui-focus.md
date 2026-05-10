# Package design: shared-room Variant 1 (input focus + viewport focus)

This document proposes a two-focus interaction model for the shared-room TUI.
It is intended as a Phase 1 direction, with the option to evolve later.

Variant 1 keeps the current "viewport + composer" structure but makes focus
explicit and leverages terminal-native mouse selection (no mouse reporting).

See also: `docs/design/pkg-ui-input.md` for input box interaction details.

## Goals

- Keep the core UX recognizable to Codex / Claude Code users.
- Make scrolling and copy/paste straightforward.
- Avoid mouse reporting (no Shift+drag requirement).
- Keep implementation maintainable (avoid terminal scrollback hacks).

## Non-goals (for now)

- Multiple panes.
- In-app mouse drag selection.
- Editing or mutating viewport content.

## Concepts

There are two focus targets:

1. **Composer focus**: user edits input (default).
2. **Viewport focus**: user navigates output history with the keyboard.

Mouse selection/copy is always handled by the terminal emulator (because mouse
reporting is not enabled).

## Keybindings

### Focus switching

- `Ctrl+O`: toggle focus between composer and viewport.

### Composer focus (default)

- Text editing uses the multiline textarea.
- `Alt+Enter`: insert newline.
- `Enter`: submit the full buffer as one message; clear composer.
- `Ctrl+C`: clear the composer buffer (CLI convention). No-op if buffer is
  already empty.
- `/quit`: exit the session.
- `Ctrl+G`: open `$EDITOR` on a temp file seeded with the composer text.
  - Cancel/non-zero exit restores pre-compose buffer.
  - On success, replace buffer with file contents (trim exactly one trailing
    newline).

### Viewport focus

Viewport is read-only.

- Arrow keys scroll the viewport line by line.
- `PgUp` / `PgDn`: scroll the viewport by half a page.
- `Home` / `End`: jump to the top / bottom of the transcript.
- `Ctrl+O`: return focus to the composer.
- `Esc`: return focus to the composer (escape hatch).
- `Ctrl+C`: no-op.
- `Ctrl+G`: open `$EDITOR` seeded with the transcript content in read-only
  mode (see below).

### PgUp / PgDn scope

`PgUp` / `PgDn` always scroll the viewport, regardless of which focus is
active. Arrow keys follow focus: in composer focus they move the cursor within
the textarea; in viewport focus they scroll the viewport.

### `Ctrl+G` in viewport focus

1. Export the transcript to a temp file (plain text, no ANSI).
2. Set the file permissions to 444 (read-only) before launching the editor.
3. Open the editor with `$EDITOR <file>` (same path resolution as composer
   Ctrl+G).
4. The export contains the full transcript, not just the visible portion.
5. Any changes the editor saves are discarded on return to the TUI.

Rationale: making the file read-only at the OS level works across editors
without requiring editor-specific flags, and communicates intent clearly.

## Focus indicator

The separator line between viewport and composer shows the current focus in
Phase 1:

- Composer focus: separator renders as `─── compose ───` (or similar).
- Viewport focus: separator renders as `─── history ───` (or similar).

Exact text and styling are implementation details.

## Output follow behavior

- When new output arrives, auto-scroll (follow) if and only if the viewport is
  already at the bottom.
- If the user has scrolled up, do not force the viewport back to bottom.
- Follow behavior is the same regardless of which focus is active.

## Implementation notes

- Mouse reporting remains disabled.
- Viewport focus is implemented by routing key events either to the textarea
  model or the viewport model depending on focus state.
- Transcript export for `Ctrl+G` in viewport focus should strip ANSI escape
  sequences to keep the editor readable.
