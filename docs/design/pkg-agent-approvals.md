# Design: Agent-level approvals (Codex app-server)

Codex app-server can ask the *client* (coderoom) to approve or deny actions
mid-turn (exec, file changes, permissions). Codex models these as **JSON-RPC
requests** emitted on stdout (they include an `id`), and it expects the client
to respond with a **JSON-RPC response** that references the same `id`.

This document defines a minimal, agent-level approval interface focused on
replicating the Codex CLI workflow in v1:

- All prompts are handled in the shared room (no rooms/private contexts yet).
- The Codex client owns protocol details and only asks the application for a
  high-level decision.

## Goals

- Handle Codex approval requests for turns started via `turn/start`.
- Keep protocol details contained inside `internal/agent/codex`.
- Provide one generic approval callback interface that the UI/session can
  implement without depending on Codex request schemas.
- Preserve ordering: the user should see approvals in the order Codex requested
  them.

## Non-goals (v1)

- Rooms/private contexts for approvals.
- Rich “apply suggested policy amendment” UX (execpolicy/network policy).
- Multi-approval selection UI (accept/decline all).
- Surfacing approval requests as `agent.Message` values (we may add this later).

## Codex protocol surface (v2)

Codex emits server-initiated JSON-RPC requests with these methods:

- `item/commandExecution/requestApproval`
- `item/fileChange/requestApproval`
- `item/permissions/requestApproval`

Each request includes a JSON-RPC `id`. The client must respond with:

```json
{"id": <same-id>, "result": { ...method-specific response... }}
```

## API design

`internal/agent` exposes an optional approval listener.

### Listener

```go
type ApprovalListener interface {
    Decide(req ApprovalRequest) (ApprovalDecision, error)
}
```

The listener:

- receives a user-facing prompt (`Ask`) plus a small set of normalized decision
  options,
- returns a normalized decision (or an error),
- does **not** need to carry protocol identifiers in the response (Codex client
  already has them from parsing the request).

### Request / decision types

```go
type ApprovalKind string

const (
    ApprovalCommandExecution ApprovalKind = "commandExecution"
    ApprovalFileChange       ApprovalKind = "fileChange"
    ApprovalPermissions      ApprovalKind = "permissions"
)

type ApprovalRequest struct {
    Kind ApprovalKind

    // Ask is a human-oriented prompt intended to be rendered directly.
    // It should be short, stable, and safe to show in the shared room.
    Ask string

    // Options is the set of normalized decisions supported by this request in v1.
    // Example: []ApprovalOption{OptionAccept, OptionDecline, OptionCancel}
    Options []ApprovalOption
}

type ApprovalOption string

const (
    OptionAccept           ApprovalOption = "accept"
    OptionAcceptForSession ApprovalOption = "acceptForSession"
    OptionDecline          ApprovalOption = "decline"
    OptionCancel           ApprovalOption = "cancel"
)

type ApprovalDecision struct {
    Choice ApprovalOption
}
```

Notes:

- `OptionCancel` means “deny and interrupt the current turn” in the Codex
  protocol (supported for command/file approvals). It is still exposed as a
  normalized option in v1.
- Permissions approvals do not have an explicit “cancel” response in the schema.
  In v1, the listener may still choose `OptionDecline`; the Codex client will
  translate that into an “empty grant” response (no additional permissions).
- `OptionAcceptForSession` is supported for command/file approvals (Codex v2).

### Default behavior (no listener)

If no listener is configured:

- Codex client responds with a safe default: `decline` (or empty grant for
  permissions).
- Codex client emits an `agent.Message{Kind: agent.MessageLog, Text: ...}`
  describing the auto-decline (so the user understands why the agent is blocked).

This makes coderoom safe-by-default even before the UI supports approvals.

## Internal algorithm (Codex client)

To avoid blocking the stdout scan goroutine, approvals are processed by a
dedicated worker loop inside the Codex client.

High-level flow:

1. Stdout scan reads a JSON-RPC message line.
2. If it is a server request (`id` present) and method is an approval request:
   - enqueue an internal `approvalJob{rpcID, method, params}` onto a **buffered**
     channel.
   - continue scanning stdout.
3. A single `approvalLoop` goroutine:
   - dequeues jobs in order,
   - formats `ApprovalRequest{Ask, Options}` for the listener,
   - calls `listener.Decide(req)` (may block),
   - translates `ApprovalDecision` to the correct schema-specific `result`,
   - writes the JSON-RPC response (`{"id":..., "result":...}`) to stdin.

This ensures:

- scanStdout never blocks on user input,
- approvals are processed sequentially,
- stdin writes remain serialized.

### Listener wiring

Codex client accepts an optional approval listener via a constructor option:

```go
func WithApprovalListener(l agent.ApprovalListener) Option
```

If unset, the client uses the safe defaults described above.

The listener should be configured before `Start()`; changing the listener after
the process is running is unspecified in v1.

### Serialization of approval responses

Approval responses share the same stdin and ordering constraints as other RPC
writes (`turn/start`, `turn/interrupt`). The implementation must serialize all
stdin writes behind a single lock (the same lock used by `writeRequest`), so
that approval responses cannot interleave with other JSON-RPC messages.

### Interrupt and pending approvals

If the user calls `/cancel` while an approval request is pending:

- The interrupt request may reach Codex before or after the approval response,
  depending on scheduling.
- V1 behavior: coderoom does not attempt to reorder or auto-drain approvals. The
  listener should still answer approvals if asked.

### Ask formatting (v1)

The Codex client formats a short `Ask` string per request kind:

- command execution: include the command string (trimmed if very long) and `cwd`
  when available.
- file change: include the set of file paths touched (and optionally counts).
- permissions: include a short summary of the requested permissions (filesystem
  and/or network) plus an optional reason, if provided.

Exact formatting is not part of the public contract; it may change as the UI
improves. It should remain safe to render in the shared room.

### Choice validation

The Codex client validates that the listener's returned `Choice` is present in
`req.Options`. If not, it falls back to the safe default for that request kind.

## Error handling

- If the listener returns an error, codex client responds with the safe default
  (decline/empty grant) and logs the error as an
  `agent.Message{Kind: agent.MessageLog, Text: ...}`.
- If stdin write fails while responding, the client treats this as fatal and the
  agent will crash (consistent with other write failures).

## Open questions

- Should approvals be surfaced as `agent.Message` (so the session/UI owns the full
  UX), rather than being handled inside the Codex client?
  - Decision: no. The approval listener is responsible for surfacing the prompt
    to the UI. We keep approval transport out of `agent.Message` in v1.
- Should we support `acceptForSession` in v1 (for command/file approvals)?
  - Decision: yes. If the backend supports it (Codex does), the listener may
    return it.
