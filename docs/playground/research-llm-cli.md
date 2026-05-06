# LLM CLI Research

## Question

Can Claude Code and Codex CLI be driven as managed subprocesses from a Go application?

---

## PTY requirement

Neither CLI works without a TTY:

- **Claude Code**: `--print` / `-p` hangs indefinitely when spawned without a TTY (issue #9026), producing no output.
- **Codex**: fails immediately with "Raw mode is not supported" (issue #1080), caused by the Ink UI library initialising before non-interactive flags are evaluated.

`creack/pty` is required for either tool.

---

## Structured output

| | Claude Code | Codex |
|---|---|---|
| Format flag | `--output-format stream-json` | `--json` |
| Format | NDJSON | NDJSON |
| Schema-constrained output | `--json-schema '<JSON Schema>'` | `--output-schema <file>` |
| Output to file | stdout redirect | `-o` / `--output-last-message` |
| Requires `--verbose` in print mode | yes | unknown |

Claude Code also supports `--input-format stream-json` for bidirectional NDJSON over stdin, though this is undocumented and does not work for multi-turn (see below).

---

## Session continuity

| | Claude Code | Codex |
|---|---|---|
| Resume last session | `claude -c` | `codex exec resume --last` |
| Resume by ID | `claude --resume <id>` | `codex exec resume <id>` |
| Session ID in output | yes, under `session_id` in JSON | yes, under `thread.id` |
| Persistence location | `~/.claude/projects/<cwd>/<id>.jsonl` | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` |
| Cross-session memory | file-based sessions | dedicated memory system (`~/.codex/memories/`) |

---

## Config suppression

| | Claude Code | Codex |
|---|---|---|
| Minimal startup flag | `--bare` | `--ignore-user-config --ignore-rules` |
| Blocks auth | yes (`--bare` and `CLAUDE_CODE_SIMPLE=1` both block auth) | unknown |
| Personal config on spawn | plugins, MCP servers, skills loaded by default | unknown |

**Claude Code finding**: there is no known way to suppress personal config (plugins, MCP servers, skills) without blocking authentication. `--bare` and `CLAUDE_CODE_SIMPLE=1` are equivalent and both break auth. The practical decision is to accept personal config loading; MCP servers appear as `needs-auth` and are inactive, and real isolation comes from the sandbox controller.

---

## Known issues relevant to subprocess integration

**Claude Code**
- `--print` hangs without TTY (issue #9026); PTY required.
- `--input-format stream-json` with `--print` exits immediately; single-shot by design, not a persistent loop.
- `--input-format stream-json` spawned from a non-shell parent forces the Bash tool's cwd to `/` (issue #46985).
- Opens `/dev/tty` and starts a blocking read loop, stealing terminal input (issue #13598).
- `--output-format stream-json` requires `--verbose` in print mode.
- PTY emits ANSI escape sequences on process exit; filter by lines starting with `{`.
- PTY uses `\r\n` line endings; trim lines before JSON parsing.

**Codex**
- Ink raw mode error in any non-TTY environment (issue #1080); PTY required.
- Session persistence can fail silently: session ID is emitted before local write completes (issue #15870).

---

## Claude Code experiment results

| Experiment | Result |
|---|---|
| PTY subprocess spawn | passed |
| `--input-format stream-json` multi-turn | failed; `--print` is single-shot |
| `--resume <session_id>` continuity | passed; context preserved across invocations |
| `CLAUDE_CODE_SIMPLE=1` config suppression | failed; blocks auth same as `--bare` |

**Agent Runtime model for Claude Code**: one process per turn, `--resume <session_id>` for continuity, personal config accepted.

---

## Codex experiment results

| Experiment | Result |
|---|---|
| PTY subprocess spawn (`codex exec`) | passed |
| `--last` session continuation | passed; context preserved across invocations |
| `CODEX_QUIET_MODE=1` config suppression | not yet run |
| app-server stdio (`codex app-server`, `poc-codex-stdio`) | passed; single process, no PTY needed, stdlib only |

**Agent Runtime model for Codex**: `app-server` over stdio is the preferred approach. Single persistent process, explicit thread ID for context, full sandbox profile exposed at session start, streaming via `item/agentMessage/delta`. Per-turn spawning with `--last` works but is fragile.

---

## Codex app-server protocol

Message schemas can be generated with:
```
npx @openai/codex app-server generate-json-schema --out ./schemas
```
Generated schemas are in `docs/playground/codex-schemas/`. Key files: `ClientRequest.json`, `ServerNotification.json`, `codex_app_server_protocol.schemas.json`.

`npx @openai/codex app-server`

stdio is the default and documented transport. WebSocket is marked unsupported. Does not require a PTY; Ink is not initialised in server mode.

JSON-RPC 2.0 over JSONL (one message per line). No `jsonrpc: "2.0"` header on the wire.

Key methods:
- `initialize` — handshake; returns server version and platform info
- `thread/start {cwd}` — create thread; returns full thread object including sandbox and permission profile
- `turn/start {threadId, input: [{type: "text", text: "..."}]}` — submit prompt

Valid input item types: `text`, `image`, `localImage`, `skill`, `mention`.

Notification sequence per turn:
```
thread/started
mcpServer/startupStatus/updated
turn/started
item/agentMessage/delta     streaming text chunks (delta field)
item/completed              full item; text at params.item.text
turn/completed              done
```
