# UI Debug Mode (`CODEROOM_DEBUG=1`)

## Goal

Provide low-friction diagnostics for UI rendering issues (layout, viewport,
scrolling) without shipping debug-only commands or overlays in the default UX.

This debug mode is intentionally scoped to answering:

- What does the UI think it is rendering?
- Is a bug caused by model state, viewport rendering, or terminal paint?

## Non-goals

- A general-purpose inspector for all internal state.
- Persistent “developer console” UI elements in normal usage.

## Activation

Debug mode is disabled by default.

Enable by setting:

`CODEROOM_DEBUG=1`

The CLI wires this into the UI model at construction time (via `ui.WithDebug`).

## Behavior

When debug mode is enabled:

- `/help` includes debug commands.
- Debug commands are accepted and executed.
- Optional debug overlays may be toggled (see `/debugrows`).

When debug mode is disabled:

- Debug commands are not shown in `/help`.
- Debug commands return a user-visible error:
  `error: debug commands disabled (set CODEROOM_DEBUG=1)`

## Debug Commands

### `/debugview`

Prints a short “what the viewport thinks it is rendering” snapshot into the
history, including:

- Viewport `YOffset` and `Height`
- Record counts
- The top few lines of `viewport.View()` (ANSI stripped)

This is intended to diagnose issues where:

- The user’s terminal display appears inconsistent with expected content.
- Scrolling appears stuck even though the model believes it is at top/bottom.

### `/debugrows`

Toggles a row-number overlay for the viewport area.

This is intended to diagnose off-by-one terminal paint issues, particularly
cases where a trailing newline causes the terminal to scroll by one row and
the first viewport line appears “missing”.

## Design Constraints

- Debug mode must not require code changes to enable during development.
- Debug features must not meaningfully complicate the non-debug code paths.
- Debug output should be low-noise and safe to paste into issues.

