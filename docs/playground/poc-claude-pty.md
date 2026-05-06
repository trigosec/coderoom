# POC: Claude Code via PTY

## Goal

Validate that Claude Code can be driven as a managed subprocess from Go using a PTY, and that its NDJSON output can be parsed reliably.

---

## Approach

Spawn Claude Code with a PTY using `creack/pty`. Pass the prompt as a CLI argument for this first step; stdin stream-json (multi-turn) is the follow-on experiment.

Flags:
- `--print`: non-interactive mode
- `--verbose`: required when using `--output-format stream-json` in print mode; omitting it causes an error and immediate exit
- `--output-format stream-json`: NDJSON output, one event per line

```
claude --print --verbose --output-format stream-json "<prompt>"
```

Note: `--bare` was expected to reduce startup noise but blocks authentication. Do not use it. Whether `CLAUDE_CODE_SIMPLE=1` as an env var avoids this is still to be investigated.

---

## Findings

**PTY approach works.** Claude Code starts, responds, and exits cleanly when spawned via PTY from Go.

**Event stream** is well-structured and predictable:

```
system (subtype: init)      session metadata, tools, plugins, model
rate_limit_event            quota and reset times
assistant                   the response message
result (subtype: success)   completion, cost, token usage, session_id
```

**`session_id`** is present in both the `system` and `result` events. This is the handle for `--resume` and the path to session continuity without respawning.

**PTY line ending quirks:**
- Lines must be trimmed (`strings.TrimSpace`) to strip carriage returns from PTY `\r\n` line endings.
- On process exit, the PTY emits ANSI escape sequences (terminal cleanup codes starting with `ESC` / byte 27). Filtering to lines starting with `{` is the cleanest way to skip all non-JSON output.

**Personal config loads** without `--bare`: MCP servers, plugins, memory paths, and skills are all present in the `system` init event. This is noise for the agent runtime but not a blocker.

---

## Success criteria

All criteria met:

1. Process starts without hanging.
2. NDJSON events are received and parsed correctly.
3. The `result` event contains the assistant response.
4. Process exits cleanly.

---

## How to run

```
cd playground/claude-pty
go run .
go run . "your prompt here"
```

---

## Next steps

1. Investigate `CLAUDE_CODE_SIMPLE=1` as an alternative to `--bare` that skips personal config without blocking auth.
2. Switch to `--input-format stream-json` on stdin for multi-turn interaction without respawning.
3. Test `--resume <session_id>` to validate session continuity across invocations.
4. Implement the Agent Runtime around this: process lifecycle, output routing to the Message Router.
