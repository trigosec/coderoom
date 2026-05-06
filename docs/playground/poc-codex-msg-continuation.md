# POC: Codex CLI message continuation

## Goal

Validate that conversational context is preserved across separate Codex invocations using session resume.

---

## Approach

Spawn a separate process per turn. Session ID is not emitted in the `--json` output stream — it is persisted to `~/.codex/sessions/` under `payload.id`. To avoid parsing the filesystem, `codex exec resume --last` resumes the most recent session.

Turn 1:
```
npx @openai/codex exec --json "What is 2 + 2?"
```

Turn 2:
```
npx @openai/codex exec resume --last --json "Multiply that result by 3."
```

The second prompt is context-dependent. If Codex answers 12, context is preserved.

Response text is extracted from `item.text` in the `item.completed` event.

---

## Success criteria

1. Turn 1 completes and an `item.completed` event with response text is received.
2. Turn 2 resumes via `--last` and completes.
3. The answer to turn 2 contains 12.

---

## How to run

```
cd playground
make pg-codex-msg-continuation
```
