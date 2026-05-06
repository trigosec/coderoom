# POC: Claude Code message continuation via --resume

## Goal

Validate that conversational context is preserved across separate Claude Code invocations using `--resume <session_id>`, and that this can serve as the session continuity model for the Agent Runtime.

---

## Background: what we tried first

The original approach was a persistent process using `--input-format stream-json` on stdin, sending multiple turns to a single Claude process. It did not work.

`--print` with `--input-format stream-json` exits immediately without emitting a `system init` event or processing any stdin input. The process reads nothing and closes the stream. This mode is single-shot by design; it does not open a persistent stdin loop.

Driving Claude's interactive mode (without `--print`) was ruled out as an alternative: the output is TUI-rendered with Ink and mixing ANSI rendering codes with NDJSON would be fragile.

---

## Approach

Spawn a separate process per turn. Pass the prompt as a CLI argument. Capture the `session_id` from the `result` event and pass it via `--resume` to the next invocation.

Turn 1:
```
claude --print --verbose --output-format stream-json "What is 2 + 2?"
```

Turn 2 (using session_id from turn 1):
```
claude --resume <session_id> --print --verbose --output-format stream-json "Multiply that result by 3."
```

The second prompt is deliberately context-dependent. If Claude answers 12, context is preserved. If not, `--resume` does not carry conversational history.

---

## Findings

**Passed.** Context is preserved across invocations via `--resume <session_id>`. The second turn correctly answered 12, confirming that conversational history is loaded from the session file.

The Agent Runtime model is validated: one process per turn, `session_id` threaded through runtime state, `--resume` for continuity.

---

## Success criteria

All criteria met:

1. Turn 1 completes and returns a `session_id`.
2. Turn 2 uses that `session_id` via `--resume` and completes.
3. The answer to turn 2 is 12.

---

## How to run

```
cd playground
make pg-claude-msg-continuation
```

---

## Next steps

1. Investigate `CLAUDE_CODE_SIMPLE=1` to suppress personal config (MCP servers, plugins) without blocking auth.
2. Model the Agent Runtime around this pattern: one process per turn, `session_id` threaded through the runtime state.
