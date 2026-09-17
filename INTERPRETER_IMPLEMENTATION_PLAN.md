# Temporary interpreter implementation plan

This file tracks the incremental implementation of GitHub issue #38. It is
temporary and must be deleted in the final boundary-enforcement commit.

Each step should leave the repository working and independently reviewable.
Run `go test ./...` before completing every step unless a narrower command is
explicitly listed in addition.

## 2. Move participant color allocation

- [x] Move participant color generation from `internal/ui/palette` to
      `internal/participant`.
- [x] Make `participant.Registry` own monotonic color allocation.
- [x] Assign color after registry validation succeeds.
- [x] Do not reuse colors after participant removal or asynchronous startup
      failure.
- [x] Remove `Color` from `session.InviteCommand`.
- [x] Remove participant palette state from the UI.
- [x] Keep departed-record, diff, and other rendering tokens in
      `internal/ui/palette`.
- [x] Move and update determinism, uniqueness, contrast, rejection, and removal
      tests.

Verification:

```text
go test ./internal/participant ./internal/session ./internal/ui/...
go test ./...
```

## 3. Make handoff input value-based

- [x] Replace `HandoffCommand.ResolveSource` with a resolved
      `session.HandoffSource` value.
- [x] Keep source selection in the current canonical room owner temporarily.
- [x] Preserve source record index and handoff audit metadata.
- [x] Update session, room, and UI tests.

Verification:

```text
go test ./internal/session ./internal/room ./internal/ui/...
go test ./...
```

## 4. Add the interpreter foundation

- [ ] Create `internal/interpreter` without importing UI or terminal packages.
- [ ] Define the narrow `SessionController` dependency.
- [ ] Add the serialized operation/event loop.
- [ ] Make the session observer enqueue and return without waiting.
- [ ] Add interpreter observers, structured events, and immutable snapshots.
- [ ] Add interpreter-owned approval DTOs and translation.
- [ ] Add lifecycle and shutdown handling.
- [ ] Add recording session and shell fakes.
- [ ] Test synchronous session callbacks and serialized `Execute` calls.
- [ ] Leave the TUI on its existing execution path for now.

Verification:

```text
go test -race ./internal/interpreter
go test ./...
```

## 5. Move basic statement execution

- [ ] Move prompt parsing and statement dispatch into the interpreter.
- [ ] Move invite, remove, cancel, policy, send, broadcast, and handoff
      translation.
- [ ] Move `/who` semantics.
- [ ] Move the room-scoped command registry.
- [ ] Move shell execution, definitions, invocation, and cancellation.
- [ ] Preserve existing syntax, routing, output, and error behavior.
- [ ] Keep the TUI on the old execution path until final cutover.

Verification:

```text
go test ./internal/interpreter ./internal/promptlang ./internal/shell
go test ./...
```

## 6. Move bounded loops

- [ ] Move active loop state and transitions into the interpreter.
- [ ] Advance loops from queued participant lifecycle events.
- [ ] Feed shell completion back through the interpreter loop.
- [ ] Preserve condition evidence and user-visible status behavior.
- [ ] Preserve cancellation, maximum-turn, stop, and crash behavior.
- [ ] Move equivalent loop tests from `internal/ui`.

Verification:

```text
go test -race ./internal/interpreter
go test ./...
```

## 7. Move staged barrier batches

- [ ] Move frozen routing plans, barrier aliases, and staged state into the
      interpreter.
- [ ] Preserve immediate and lifecycle-delayed dispatch.
- [ ] Preserve target departure, partial delivery, and handoff output/idle
      ordering.
- [ ] Implement `TakeStageForEdit() (string, bool)`.
- [ ] Implement `DiscardStage() bool`.
- [ ] Implement `InterruptAndDispatchStage() bool`.
- [ ] Process all stage operations atomically on the interpreter loop.
- [ ] Reject operations cleanly after shutdown.
- [ ] Ensure accepted synchronous requests resolve or observe interpreter
      completion.
- [ ] Add auto-dispatch/edit/discard races, duplicate interrupt, and shutdown
      tests.

Verification:

```text
go test -race ./internal/interpreter
go test ./...
```

## 8. Cut the TUI over

- [ ] Construct and observe the interpreter from the application composition
      root.
- [ ] Submit all prompt-language input through `Interpreter.Submit`.
- [ ] Render interpreter events and snapshots.
- [ ] Run synchronous stage operations inside `tea.Cmd`.
- [ ] Remove UI-owned registry, shell execution, loop state, and barrier
      coordination.
- [ ] Remove direct UI session observation, commands, and snapshot queries.
- [ ] Remove UI imports of `internal/session` and `internal/agent`.
- [ ] Remove obsolete UI tests and helpers only after equivalent interpreter
      coverage exists.

Verification:

```text
go test -race ./internal/interpreter ./internal/ui/... ./internal/session
go test ./...
```

## 9. Enforce the boundary and clean up

- [ ] Add a normal architecture test using `go list`.
- [ ] Reject transitive interpreter dependencies on `internal/ui`, Bubble Tea,
      Bubbles, and Lip Gloss.
- [ ] Reject direct UI imports of `internal/session` and `internal/agent`.
- [ ] Remove transitional adapters, duplicate execution paths, and obsolete
      APIs.
- [ ] Update implementation-status wording in the design documents.
- [ ] Delete this temporary plan file in this commit.

Verification:

```text
go test -race ./internal/interpreter ./internal/ui/... ./internal/session
go test ./...
git diff --check
```
