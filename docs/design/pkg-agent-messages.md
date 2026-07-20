# Package analysis: Codex item lifecycle API (v2)

This document analyzes the **Codex app-server v2 "item lifecycle" API** as represented by the JSON Schemas in `codex-json-schema/v2/`.

Codex emits structured units called **ThreadItems** and reports their lifecycle via notifications such as `item/started` and `item/completed`. Some item types also have additional delta-style notifications (e.g. command output deltas).

The current product problem: coderoom is only translating a small subset of notifications into `internal/agent.Message` (agent text deltas, reasoning deltas, turn completion). As a result, **action/context items** such as `commandExecution` (and their `aggregatedOutput`) can be silently dropped from the transcript.

This doc inventories the API surface and proposes how to represent these items in agent-agnostic types.

---

## Representation design

### Typed domain structs

The right abstraction boundary is not between "generic container" and "Codex-specific struct" — it is between **wire format** and **domain concept**. Define named structs in `internal/agent` that represent concepts (command execution, file change, tool call), not Codex wire types. The Codex adapter maps `CommandExecutionThreadItem` → `agent.Command`; a future adapter maps its own wire format to the same structs.

Codex-specific fields that coderoom has no use for (`processId`, `source`, `commandActions`) are dropped at the adapter boundary and never reach `internal/agent`.

For low-relevance item types that don't need rich rendering (`imageView`, `enteredReviewMode`, `contextCompaction`, etc.), `GenericTool` serves as a single placeholder (see domain types below).

### Message model

`agent.Message` is designed around three orthogonal concepts:

- **`StreamID`** — which logical stream this message belongs to; messages sharing an ID form a stream
- **`Mode`** — the streaming lifecycle signal for this message
- **`Content`** — the typed payload, sealed via an unexported interface method

```go
// StreamID identifies a logical message stream. Messages sharing an ID form one
// stream. The consumer uses it for grouping only — never parse or construct it
// outside the adapter.
type StreamID string

type Mode int

const (
    ModeStream Mode = iota // fragment; a ModeFlush with the same content type follows
    ModeFlush              // stream is closed; carries the same content type as ModeStream
    ModeSingle             // standalone message; not part of a stream
)

// MessageContent is implemented only by types in this package.
type MessageContent interface {
    content()
}

type Message struct {
    StreamID StreamID
    Mode     Mode
    Content  MessageContent
}
```

### Mode semantics

`Mode` is the sole lifecycle signal:

| Mode | Content | Meaning |
|---|---|---|
| `ModeStream` | Output, Reasoning, … | fragment; a `ModeFlush` with the same content type follows |
| `ModeFlush` | same type as stream (zero-value payload) | this stream is closed |
| `ModeSingle` | Log | complete, standalone; not part of a stream |

**`ModeFlush` carries the same content type as its stream's `ModeStream` messages**, with a zero-value payload. This lets consumers dispatch entirely on content type without inspecting `Mode` to determine what kind of stream is closing.

**Output stream end** is signalled by `Output + ModeFlush` on the same
`StreamID` as the corresponding `Output + ModeStream` fragments.

**Notice turn-end** (from `Agent.SendNotice`) is also signalled by
`Output + ModeFlush`, but on a dedicated synthetic stream
(`codex:notice-turn`) rather than a visible output item stream.

**Reasoning segment end** is signalled by `Reasoning + ModeFlush`. Multiple reasoning segments can occur within a single turn, each with a distinct `StreamID`.

**The adapter never accumulates.** It emits raw fragments on `ModeStream` and the typed zero-value flush on `ModeFlush`.

### Accumulation via `Message.Accumulate`

Accumulation is explicit and uniform across all content types:

```go
// Accumulate merges next into m, returning the updated message.
// Used by consumers to build up stream state across ModeStream messages.
// Returns an error if the StreamIDs or content types are incompatible.
// When next is ModeFlush the returned message carries the same content as m
// (the flush payload is empty) with Mode set to ModeFlush.
func (m Message) Accumulate(next Message) (Message, error)
```

The consumer pattern is identical regardless of content type:

```go
current, _ := agent.Read()
for current.Mode == ModeStream {
    next, err := agent.Read()
    // ...
    current, err = current.Accumulate(next)
    // ...
}
// current.Mode == ModeFlush: stream is done, current holds final state
```

`Accumulate` merge semantics per content type:

- **`Output`** — concatenate the `Text` attribute
- **`Reasoning`** — concatenate the `Text` attribute

Returning an error on StreamID or content type mismatch makes stream corruption explicit.

A flush carries a zero-value payload, so `StreamMessage.Accumulate(FlushMessage)` preserves the accumulated content and sets `Mode` to `ModeFlush`:

```
// Output (text): accumulate then flush
{ID: "codex:output:<turnId>:<itemId>", Mode: ModeStream, Content: Output{Text: "Hel"}}
{ID: "codex:output:<turnId>:<itemId>", Mode: ModeStream, Content: Output{Text: "lo"}}
{ID: "codex:output:<turnId>:<itemId>", Mode: ModeFlush,  Content: Output{}}

// After Accumulate:
{ID: "codex:output:<turnId>:<itemId>", Mode: ModeFlush, Content: Output{Text: "Hello"}}
```

Sequences:

```
// Output (text): item-scoped deltas and same-stream flush
{ID: "codex:output:<turnId>:<itemId>", Mode: ModeStream, Content: Output{Text: "Hel"}}
{ID: "codex:output:<turnId>:<itemId>", Mode: ModeStream, Content: Output{Text: "lo"}}
{ID: "codex:output:<turnId>:<itemId>", Mode: ModeFlush,  Content: Output{}}

// Reasoning (two segments): each ends with Reasoning+ModeFlush
{ID: "codex:reasoning:msg_1",  Mode: ModeStream, Content: Reasoning{Text: "..."}}
{ID: "codex:reasoning:msg_1",  Mode: ModeFlush,  Content: Reasoning{}}
{ID: "codex:reasoning:msg_2",  Mode: ModeStream, Content: Reasoning{Text: "..."}}
{ID: "codex:reasoning:msg_2",  Mode: ModeFlush,  Content: Reasoning{}}

// Log (standalone)
{ID: "codex:log",              Mode: ModeSingle, Content: Log{Text: "npm warn ..."}}
```

### Stream IDs

StreamIDs are constructed by adapter-internal helpers in `stream_ids.go` and are never parsed or constructed by consumers.

**Consumer rule: compare StreamIDs for equality only.** Never branch on prefixes, suffixes, or substrings. All semantic behavior — stream closure, reasoning segment close, log routing — is driven by the content type and mode of the message, not by inspecting the ID string. The `codex:output:` / `codex:reasoning:` prefixes visible in the examples are an implementation detail of the Codex adapter and must not leak into consumer logic.

```go
func outputStreamID(turnID, itemID string) StreamID // "codex:output:<turnID>:<itemID>"
func reasoningStreamID(itemID string) StreamID // "codex:reasoning:<itemID>"

const logStreamID         = StreamID("codex:log")
const noticeRelayStreamID = StreamID("codex:notice-relay")
const noticeTurnStreamID  = StreamID("codex:notice-turn")
```

### Content types

All content types implement `MessageContent` via an unexported method:

```go
type Output    struct{ Text string }
type Reasoning struct{ Text string }
type Log       struct{ Text string }

func (Output) content()    {}
func (Reasoning) content() {}
func (Log) content()       {}
```

**Value receivers are deliberate.** All payload fields (`string`, `[]byte`, `[]FileChange`) are reference types in Go — copying a struct header is always cheap regardless of content size. Value receivers avoid nil pointer concerns and keep type switch cases free of `*`. If pointer receivers are ever introduced, all dispatch sites must be updated: `case Command` and `case *Command` are distinct and a mismatch fails silently.

`messageFromEnvelope` in the Codex adapter constructs `StreamID` values and returns `Message` for all Codex notifications.

### Proposed domain types (high-relevance items)

`ToolStatus` is retained on structs where the wire protocol carries an outcome status (patch apply result, MCP call outcome). It is a **domain concern** — distinct from `Mode`, which is a **pipeline concern**.

```go
type ToolStatus string

const (
    ToolStatusInProgress ToolStatus = "inProgress"
    ToolStatusCompleted  ToolStatus = "completed"
    ToolStatusFailed     ToolStatus = "failed"
    ToolStatusDeclined   ToolStatus = "declined"
)

type Command struct {
    Command  string
    Cwd      string
    Output   string // stdout+stderr delta fragment
    ExitCode *int   // nil on intermediate ModeStream messages; set on the final one (from item/completed)
}

func (Command) content() {}

type FileChange struct {
    Path       string
    Diff       string
    ChangeKind string // "add" | "delete" | "update"
}

type FileChangeSet struct {
    Status  ToolStatus   // "inProgress" on item/started; final status on item/completed
    Changes []FileChange // partial patch set per ModeStream; complete on the final one
}

func (FileChangeSet) content() {}

type MCPToolCall struct {
    Server    string
    Tool      string
    Arguments []byte // raw JSON; schema varies per tool
    Status    ToolStatus
    Result    []byte // raw JSON or nil
    Error     string // non-empty on failure
}

func (MCPToolCall) content() {}

type DynamicToolCall struct {
    Tool    string
    Status  ToolStatus
    Success *bool
}

func (DynamicToolCall) content() {}

type WebSearch struct {
    Query string
}

func (WebSearch) content() {}

type ImageGeneration struct {
    Status        ToolStatus
    SavedPath     string
    RevisedPrompt string
}

func (ImageGeneration) content() {}

type GenericTool struct {
    Type  string // wire type name, for display only
    Title string
}

func (GenericTool) content() {}
```

`collabAgentToolCall` is omitted for now; it requires deeper thought around how coderoom represents cross-agent coordination that it is itself orchestrating.

---

## Wire notifications (inventory)

### Item lifecycle notifications

From:
- `codex-json-schema/v2/ItemStartedNotification.json`
- `codex-json-schema/v2/ItemCompletedNotification.json`

Shape (high level):
- `threadId` (string)
- `turnId` (string)
- `startedAtMs` / `completedAtMs` (int64)
- `item` (`ThreadItem` union, see below)

### Additional item-adjacent notifications (non-exhaustive)

These notifications are separate from the `item/started` / `item/completed` lifecycle, and may be emitted while an item is running:

- `codex-json-schema/v2/CommandExecutionOutputDeltaNotification.json`
  - `itemId`, `delta`, `threadId`, `turnId`
- `codex-json-schema/v2/FileChangePatchUpdatedNotification.json`
  - `itemId`, `changes[]`, `threadId`, `turnId`
  - NOTE: `FileChangeOutputDeltaNotification` exists but is marked deprecated in the schema.

Implication: some item types are better represented as **(started → zero+ deltas → completed)** rather than only started/completed snapshots.

---

## ThreadItem union (v2)

The `item` field in `ItemStartedNotification` / `ItemCompletedNotification` is a `ThreadItem` union. In `codex-json-schema/v2/ItemStartedNotification.json`, `ThreadItem.oneOf` includes at least:

- `userMessage`
- `hookPrompt`
- `agentMessage`
- `plan`
- `reasoning`
- `commandExecution`
- `fileChange`
- `mcpToolCall`
- `dynamicToolCall`
- `collabAgentToolCall`
- `webSearch`
- `imageView`
- `imageGeneration`
- `enteredReviewMode`
- `exitedReviewMode`
- `contextCompaction`

The rest of this document summarizes the schema-defined fields for each type, focusing on what is relevant to "action/context in the transcript".

---

## Item type details

### `commandExecution`

Source: `CommandExecutionThreadItem` definition inside `ItemStartedNotification.json` / `ItemCompletedNotification.json`.

Required fields:
- `type`: `"commandExecution"`
- `id`: string
- `status`: `CommandExecutionStatus` (enum includes at least `inProgress` in schema; also used with completed/failed in practice)
- `command`: string
- `cwd`: absolute path string (schema: `AbsolutePathBuf`)
- `commandActions`: array of `CommandAction` (best-effort parsed actions)

Optional fields:
- `aggregatedOutput`: string|null (combined stdout+stderr)
- `durationMs`: int64|null
- `exitCode`: int32|null
- `processId`: string|null
- `source`: enum `agent|userShell|unifiedExecStartup|unifiedExecInteraction`

Related notifications:
- `CommandExecutionOutputDeltaNotification`: streaming output deltas keyed by `itemId`.

Transcript relevance:
- High. This is often the only place where execution evidence lives (e.g. `git diff` output).

### `fileChange`

Source: `FileChangeThreadItem` definition inside `ItemStartedNotification.json` / `ItemCompletedNotification.json`.

Required fields:
- `type`: `"fileChange"`
- `id`: string
- `status`: `PatchApplyStatus` (enum defined in schema)
- `changes`: array of `FileUpdateChange`

`FileUpdateChange` fields (from `FileChangePatchUpdatedNotification.json`):
- `path`: string
- `diff`: string
- `kind`: `{type: add|delete|update, move_path?: string|null}`

Related notifications:
- `FileChangePatchUpdatedNotification`: emits `changes[]` for `itemId` (useful as an incremental view).
- `FileChangeOutputDeltaNotification`: exists but is marked deprecated.

Transcript relevance:
- High. File diffs/paths are critical context for "what changed".

### `mcpToolCall`

Source: `McpToolCallThreadItem` definition inside `ItemStartedNotification.json`.

Required fields:
- `type`: `"mcpToolCall"`
- `id`: string
- `server`: string
- `tool`: string
- `arguments`: (schema allows any JSON)
- `status`: `McpToolCallStatus` (enum in schema)

Optional fields:
- `durationMs`: int64|null
- `result`: `McpToolCallResult`|null
- `error`: `McpToolCallError`|null
- `mcpAppResourceUri`: string|null

Transcript relevance:
- Medium to high. This is a "tool run" with arguments and structured result/error.

### `dynamicToolCall`

Source: `DynamicToolCallThreadItem` definition inside `ItemStartedNotification.json`.

Required fields:
- `type`: `"dynamicToolCall"`
- `id`: string
- `tool`: string
- `arguments`: (any JSON)
- `status`: `DynamicToolCallStatus` (enum in schema)

Optional fields:
- `namespace`: string|null
- `durationMs`: int64|null
- `success`: bool|null
- `contentItems`: array|null of `DynamicToolCallOutputContentItem`

Transcript relevance:
- Medium. Similar to `mcpToolCall` but "dynamic" namespace/tooling.

### `collabAgentToolCall`

Source: `CollabAgentToolCallThreadItem` definition inside `ItemStartedNotification.json`.

Required fields:
- `type`: `"collabAgentToolCall"`
- `id`: string
- `tool`: `CollabAgentTool` enum (`spawnAgent|sendInput|resumeAgent|wait|closeAgent`)
- `status`: `CollabAgentToolCallStatus` enum (`inProgress|completed|failed`)
- `senderThreadId`: string
- `receiverThreadIds`: array of strings
- `agentsStates`: map threadId → `CollabAgentState` (status + optional message)

Optional fields:
- `prompt`: string|null
- `model`: string|null
- `reasoningEffort`: enum|null

Transcript relevance:
- Medium. Important for multi-agent coordination visibility; may be noisy unless collapsed.

### `webSearch`

Source: `WebSearchThreadItem` definition inside `ItemStartedNotification.json`.

Required fields:
- `type`: `"webSearch"`
- `id`: string
- `query`: string

Optional fields:
- `action`: `WebSearchAction`|null (schema-defined; shape varies)

Transcript relevance:
- Medium. Usually should show the query; results may be elsewhere or embedded.

### `imageView`

Source: `ImageViewThreadItem` definition inside `ItemStartedNotification.json`.

Required fields:
- `type`: `"imageView"`
- `id`: string
- `path`: absolute path

Transcript relevance:
- Low to medium. Useful as context ("looked at file X.png").

### `imageGeneration`

Source: `ImageGenerationThreadItem` definition inside `ItemStartedNotification.json`.

Required fields:
- `type`: `"imageGeneration"`
- `id`: string
- `status`: string
- `result`: string

Optional fields:
- `revisedPrompt`: string|null
- `savedPath`: absolute path|null

Transcript relevance:
- Medium. Usually want prompt/result reference; details TBD.

### `enteredReviewMode` / `exitedReviewMode`

Source: corresponding ThreadItem definitions inside `ItemStartedNotification.json`.

Required fields:
- `type`: `"enteredReviewMode"` or `"exitedReviewMode"`
- `id`: string
- `review`: string

Transcript relevance:
- Low. Mostly UI/flow state.

### `contextCompaction`

Source: `ContextCompactionThreadItem` definition inside `ItemStartedNotification.json`.

Required fields:
- `type`: `"contextCompaction"`
- `id`: string

Transcript relevance:
- Low. Useful as a marker for debugging, but not typically user-facing.

---

## Non-tool conversational types (for completeness)

These exist in the ThreadItem union but are already surfaced via dedicated streaming notifications in coderoom today:

- `agentMessage` (has `text`)
- `reasoning` (has `summary[]` and `content[]` arrays in the item snapshot, plus `item/reasoning/*` delta notifications)
- `plan` (has `text`)
- `userMessage` / `hookPrompt`

coderoom will continue treating chat/reasoning via delta notifications and handle "action/context items" via lifecycle notifications, consistent with the streaming model described above.
