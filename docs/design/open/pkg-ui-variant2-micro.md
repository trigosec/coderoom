# Package design: shared-room Variant 2 (micro-based rich client)

This document explores replacing the Bubble Tea UI with a rich terminal client
built on the `micro` editor project:

- https://micro-editor.github.io/

The intent is to get a best-in-class terminal editing experience “for free”
(selection, scrolling, search, keybindings, multi-line editing) by building on
an existing editor, rather than recreating those features in Bubble Tea.

This is an optional Phase 2+ exploration, not a Phase 1 requirement.

## Goals

- Provide a familiar, powerful terminal UX for:
  - multi-line composition
  - navigation/search in output
  - selection and copy/paste
- Reduce bespoke UI complexity by leveraging a mature editor core.

## Non-goals (for now)

- Committing to micro as a hard dependency.
- Shipping editor plugins or deep editor customization in Phase 1.

## What “micro-based” could mean

There are at least two interpretations:

1. **Embed micro as the entire UI**:
   - The app becomes “an editor with panels/buffers” (shared room + per-agent
     private buffers).
2. **Use micro as a component**:
   - Keep Code Room’s controller/runtime, but use micro for the main view and
     input.

Either approach likely implies stopping (or heavily reducing) Bubble Tea usage.

## Potential UX shape

- One buffer per context:
  - shared transcript (read-only or append-only)
  - per-agent private channels (separate buffers)
  - compose buffer (editable)
- Keybindings and selection behave like an editor (predictable across terminals).
- Output streaming appends to buffers; the editor handles scrolling and selection
  naturally.

## Pros

- Rich selection, navigation, and search are “built in”.
- Strong multi-line editing UX (undo, movement, word ops).
- Potentially a more recognizable experience for users coming from terminal
  editors.

## Cons / risks

- Big architectural shift: UI becomes an editor application.
- Larger dependency surface area and maintenance burden.
- Integration complexity:
  - how to drive micro programmatically
  - how to maintain a stable API boundary between session/controller and UI
- Harder to keep the UI minimal; risk of “building an IDE in the terminal”.

## Scope of the spike

The goal of the spike is to validate UX quality, not to commit to an
implementation path. How micro is ultimately integrated — using selected
packages, forking the project, or another approach — is a separate decision
that depends on what the spike reveals.

The spike should answer one question: **does micro demonstrate that a terminal
app can provide great UX for the pain points Variant 1 leaves open?**

Those pain points are:

- Text selection and copy without Shift+drag or in-app selection machinery.
- Search and navigation in large output transcripts.
- Rich multi-line composition (undo, word motions, cursor movement).

## Spike evaluation criteria

Consider the spike successful if micro’s UX model covers all three pain points
convincingly in a realistic session (streaming output, multi-agent transcript,
multi-line compose). Consider it unsuccessful if any of:

- Selection or copy does not work reliably over SSH.
- Search over a large transcript is slow or visually disruptive.
- Multi-line composition feels significantly worse than a native terminal editor.

Implementation complexity and integration architecture are **out of scope** for
the spike evaluation. Those are decided only after the UX question is settled.

## Recommendation

Treat Variant 2 as a UX proof-of-concept spike before making any architectural
commitment. Only if the spike validates the UX should the team evaluate
integration paths (packages, fork, or otherwise) and compare the maintenance
cost against Variant 1.

