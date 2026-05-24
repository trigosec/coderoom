# UI design (open): expand/collapse file-change diffs in history mode

Status: open / parking lot

This document proposes a keyboard-driven way to expand or collapse file-change
diffs while browsing history (`Ctrl+O` viewport focus). The motivation is to
support “follow the patch in the UI” workflows without always dumping large
diffs into the main transcript view.

## Background

- The main transcript view is optimized for scanning conversation.
- File-change records can contain large diffs that are useful to inspect in
  context, but they can also overwhelm the scrollback.
- Today, the viewport view renders a file-change record as:
  - a `files:` summary list
  - a short diff body preview (first N lines)
  - a hint to open the full transcript (`Ctrl+G`)

## Goals

- Allow inspecting a file-change diff “in place” while in history mode.
- Keep the default transcript view compact (preview + hint).
- Make the interaction reversible (collapse restores a scannable history).
- Avoid introducing an import-cycle-prone dependency from record rendering to
  higher-level UI packages.

## Non-goals

- Mouse-based expand/collapse.
- Full “copy mode” / selection implementation (see `docs/design/open/pkg-ui-selection.md`).
- Per-hunk folding or incremental diff pagination.

## Proposed interaction model

### Focus modes

This feature is only active when the viewport has focus (“history mode” in
`docs/design/pkg-ui-focus.md`).

If history mode currently always snaps to the top of the viewport, this feature
is a poor fit until focus/position handling improves (see “Prerequisites”).

### Keybinding: `Ctrl+D`

In history mode:

- `Ctrl+D` toggles **diff expansion** for the **most recent file-change record**
  (or, if the UI has a selected record concept, the selected record).
- If there is no file-change record in the buffer, no-op.

Rationale:

- The main transcript view has no “current record” selection, so `Ctrl+D` must
  have a deterministic target in viewport focus.
- “Most recent file-change record” matches the common workflow: an agent emits a
  patch, and the user wants to inspect it immediately.

### Rendering: collapsed vs expanded

For a `KindFileChange` record in viewport rendering (`RenderViewport`):

- Collapsed:
  - `files:` list
  - preview body (first N lines)
  - `(+N more; …)` hint
- Expanded:
  - `files:` list
  - full diff body in-place (all lines)
  - optional one-line hint: `(Ctrl+D collapse)`

Notes:

- Expanded rendering should still wrap to viewport width (no horizontal
  scrolling).
- “Full body” means all `FormatFileChangeBody(...)` lines, not only the visible
  portion.

## Prerequisites / dependencies

### Restore viewport position on `Ctrl+O`

If `Ctrl+O` enters history mode but always places the cursor/viewport at the top
of the transcript, then expanding diffs is hard to use because the user must
scroll down to find the relevant record every time.

Preferred behavior:

- `Ctrl+O` should preserve the current viewport scroll position when switching
  focus.
- Optionally, store and restore the last known “history position” separately
  from “compose-follow position”.

### Stable record identity

To store per-record expansion state, each record needs a stable identifier that
survives re-rendering and resize reflows. Options:

1. Record index in the record slice (simple, but brittle if records are deleted
   or inserted in the middle).
2. A monotonically increasing `RecordID` assigned at record creation (preferred).

## Data model sketch

In the room/history model:

- Track `expandedFileChanges map[RecordID]bool`.
- When rendering a `KindFileChange` record in viewport mode, consult this map to
  decide whether to preview or render the full body.

Record rendering should remain pure with respect to global UI state: it should
only depend on the supplied `RenderContext` and a small per-record expansion
signal, not on importing higher-level UI packages.

## Edge cases

- Multiple diffs in one conversation:
  - `Ctrl+D` toggles the most recent one; older diffs can be toggled by entering
    history mode and using an explicit selection mechanism (future) or a
    command (future).
- Huge diffs:
  - Expanded mode can make the viewport jump; consider preserving the current
    top line when toggling to reduce disorientation.
- Streaming tool output:
  - If a file-change record is still pending/streaming, expanding should be a
    no-op until the body exists (or expand once content arrives).

## Alternatives considered

- Always show full diffs in the main transcript:
  - Pros: simple, aligns with “patch in UI” expectation.
  - Cons: dominates scrollback and makes conversation hard to follow.
- Global expand/collapse-all:
  - Unambiguous target in a selection-less UI, but very easy to blow up the
    transcript in long sessions.
- Open transcript in editor (`Ctrl+G`) only:
  - Great for deep review/search, but it breaks the “stay in UI” flow.

## Open questions

- Should `Ctrl+D` target “most recent file-change record” or “nearest above the
  viewport bottom”?
- Do we want a discoverable command form (e.g. `/diff expand`) in addition to a
  keybinding?
- Should expanded diffs suppress the `(+N more …)` hint entirely?

