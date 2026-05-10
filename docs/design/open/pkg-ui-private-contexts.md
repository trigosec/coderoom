# Package design: terminal-native transcript with private contexts (open)

This document explores how to combine a terminal-native transcript architecture
(terminal scrollback owns history, native selection/copy) with Code Room’s
shared-room + per-agent “private” conversations.

It is a design exploration, not a v1 commitment.

## Background

Code Room’s channel model separates:

- **Shared room**: visible to all agents + the user; primary coordination space.
- **Private agent channel**: user ↔ single agent; includes operational detail
  (reasoning/approvals); not visible to other agents.

In a classic fullscreen TUI, the app can render separate tabs/panes. In a
terminal-native transcript architecture, the terminal emulator owns scrollback
and native text selection. This creates constraints on how “private” contexts
can be represented on screen.

## Terminology: two meanings of “private”

When we say “private messages”, there are two distinct concerns:

1. **Agent privacy**: messages are not shown to other agents (routing/privacy at
   the session layer).
2. **UI separation**: the user can focus on one context (shared vs a specific
   agent) without interleaved output clutter.

This document focuses on (2) while preserving (1).

## Requirements

- Preserve terminal-native:
  - scrollback / mouse wheel scrolling
  - drag-to-select copy
- Keep a persistent bottom composer.
- Support:
  - one shared conversation
  - per-agent private conversations
- Avoid forcing Shift+drag selection.

## Non-goals (for now)

- Multi-pane dashboards.
- Mouse-driven in-app selection.
- Strong secrecy against someone who can see the user’s terminal history (the
  user is always the authority; “private” is about routing and UX separation).

## Option A: single terminal transcript, in-band routing markers

All output is appended to a single terminal transcript. Messages are tagged
visually by channel:

- `[shared] …`
- `[@ada private] …`

### UX

- The user sees all contexts interleaved in one scrollback.
- The composer always sends to the “active target” (default: shared). The user
  can retarget:
  - `@ada …` sends private to ada
  - plain text sends shared (or broadcasts)

### Pros

- Simple implementation: one transcript stream.
- Native scrollback/selection preserved (no mouse reporting needed).
- Matches a “chat log” mental model.

### Cons

- Shared room becomes noisy with private operational output.
- Context switching is cognitive (you must parse tags).
- Not truly “UI-private”: everything is visible in the same scrollback.

### When to choose

Choose this if “private” means primarily “not visible to other agents”, and the
user accepts a single combined terminal log.

## Option B: focus switching (single terminal, separate transcripts by focus)

The application maintains separate transcripts internally:

- shared transcript
- per-agent private transcript(s)

The terminal shows only the currently focused transcript. The user can switch
focus:

- `/focus shared`
- `/focus @ada`

### Rendering on focus switch

On switching focus, the UI performs a best-effort “screen reset” and re-renders
the focused transcript:

1. Clear the visible terminal screen (best effort; uses terminal control
   sequences).
2. Print the focused transcript (append-only output for that context).
3. Draw the bottom chrome (composer + toolbox/status).

This keeps the interaction model recognizable (transcript + bottom composer),
while providing visual separation between contexts.

### UX

- The screen always shows one context at a time (shared or one agent).
- The composer sends to the active focus by default.
- Switching focus is explicit and discoverable.

### What happens to scrollback

Terminal scrollback is per-terminal-session. When the app “switches focus” it
must re-render the focused transcript into the terminal output stream.

Implication:

- Switching focus effectively creates a new segment of terminal history that
  contains a reprint of the transcript for that focus.
- The terminal’s scrollback will include prior focus renders (duplicates).

This keeps terminal-native selection, but the scrollback is no longer a clean,
single, deduplicated history.

### Pros

- Keeps shared room clean when user is focusing on it.
- Strong UX separation without fullscreen panes.
- No need for OS-level terminal integration.

### Cons

- Terminal scrollback contains repeated content after focus switches.
- “Global history” across focuses is awkward; the terminal is not aware of
  logical channels.
- Large transcripts may be slow/noisy to reprint on every switch; mitigation may
  include reprinting only the last N records and relying on `/pager` for full
  history.

### When to choose

Choose this if UI separation is important and you can accept scrollback
duplication as the cost of staying terminal-native without panes.

## Option C: separate terminal buffers per context (tmux / terminal integration)

Each context gets its own terminal buffer/scrollback:

- shared room runs in one terminal session
- each agent private channel runs in its own terminal tab/window or tmux
  window/pane

### UX

- `/open @ada` opens a new tab/window/pane for ada’s private channel.
- The shared room remains in the original terminal.
- Each context has independent native scrollback and selection.

### Pros

- Best of both worlds:
  - true UI separation
  - clean terminal scrollback per context (no duplication)
- Scales to many agents without interleaving.

### Cons

- Requires integration:
  - tmux detection/control, or
  - launching new terminal windows/tabs, which is OS/terminal-specific
- Harder to make cross-platform.

### When to choose

Choose this if terminal-native UX is non-negotiable and clean separation is
critical, and you are willing to build/maintain integration code.

## Recommendation

For a terminal-native transcript architecture, the most practical progression
is:

1. Start with **Option A** (in-band tags) if simplicity is the priority.
2. If clutter becomes a real pain, consider **Option B** (focus switching) as an
   intermediate step.
3. If you want “true tabs” with clean per-context scrollback, invest in
   **Option C** (tmux/terminal integration).

## Open questions

1. Product intent: does “private channel” mean only “not shown to other agents”,
   or does it also mean “separate user workspace context”?
2. Switching cost: how often do users switch between shared and private views in
   typical workflows?
3. Integration scope: are we willing to require or optionally support tmux for a
   high-quality multi-context UX?
