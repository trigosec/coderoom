# POC: Codex CLI via app-server (stdio)

## Goal

Validate that Codex can be driven over its app-server stdio transport using JSON-RPC 2.0, enabling a single persistent process with a clean, secure communication channel.

---

## Transport choice

The app-server supports three transports: `stdio` (default), WebSocket, and Unix socket.

WebSocket (`ws://`) is marked as unsupported in the official documentation. The stdio transport is the documented, supported path: the server reads JSONL from stdin and writes JSONL to stdout. No port binding, no socket accessible to other processes — the communication channel is private to the parent-child process relationship.

```
npx @openai/codex app-server
```

No `--listen` flag needed; stdio is the default.

---

## Protocol

JSON-RPC 2.0 over newline-delimited JSON (JSONL). No `jsonrpc: "2.0"` header on the wire. Each message is one line.

Flow:
1. `initialize` — handshake with client info and capabilities
2. `thread/start {cwd}` — create a persistent thread, receive `thread.id`
3. `turn/start {threadId, input: [{type: "text", text: "..."}]}` — submit prompt
4. Read notifications until `turn/completed`
5. Repeat step 3 for subsequent turns on the same thread

Valid input item types: `text`, `image`, `localImage`, `skill`, `mention`.

Notification sequence per turn:
```
thread/started
mcpServer/startupStatus/updated
turn/started
item/agentMessage/delta     streaming text chunks (delta field)
item/completed              full item; text at params.item.text
turn/completed
```

---

## Findings

**Passed** on both WebSocket and stdio transports. Single process, context preserved across turns. Second turn answered "4 × 3 = 12."

No external dependencies required — stdlib `os/exec` with stdin/stdout pipes handles the transport. `gorilla/websocket` removed.

---

## Success criteria

All criteria met:

1. app-server starts and communicates over stdio.
2. `initialize` and `thread/start` succeed.
3. Both turns complete via `turn/completed`.
4. Second turn response contains 12.

---

## How to run

```
cd playground
make pg-codex-ws
```
