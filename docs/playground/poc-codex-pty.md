# POC: Codex CLI via PTY

## Goal

Validate that Codex CLI can be driven as a managed subprocess from Go using a PTY, and that its NDJSON output can be parsed reliably. Mirrors `poc-claude-pty.md` for direct comparison.

---

## Approach

Spawn Codex via PTY using `creack/pty`. Use `--json` for NDJSON output.

```
npx @openai/codex exec --json "<prompt>"
```

---

## Findings

**Passed.** Codex starts cleanly via PTY and emits well-structured NDJSON.

Event stream:

```
turn.started      turn begins
item.completed    assistant response; text at item.text
turn.completed    completion with token usage
```

Notably simpler than Claude Code: no system init event, no rate limit event, no session metadata in the output stream.

**Session ID is not emitted in the `--json` output stream.** It is persisted to disk under `~/.codex/sessions/YYYY/MM/DD/rollout-<id>.jsonl` as a `session_meta` entry. The ID is also encoded in the filename. For session continuation, `codex exec resume --last` avoids needing to extract the ID from the stream or the filesystem.

---

## Success criteria

All criteria met:

1. Process starts without the Ink raw mode error.
2. NDJSON events are received and parsed.
3. `turn.completed` is received with token usage.
4. Process exits cleanly.

---

## How to run

```
cd playground
make pg-codex-pty
```

---

## Next steps

1. Test session continuation via `codex exec resume --last`.
2. Test config suppression with `--ignore-user-config --ignore-rules`.
3. Compare with Claude Code and update the research doc.
