# POC: Suppressing Codex personal config

## Goal

Validate whether Codex CLI config loading can be suppressed without blocking authentication. Mirrors `poc-claude-simple-env.md`.

---

## Approach

Spawn Codex with `CODEX_QUIET_MODE=1` and inspect the `thread.started` event for signs of user config (memories, rules). Also test `--ignore-user-config` and `--ignore-rules` flags if the env var has no effect.

From the research, `CODEX_QUIET_MODE=1` was reported not to prevent the Ink raw mode error — but with PTY in place, the raw mode issue is resolved. Whether it suppresses config loading is untested.

---

## Success criteria

1. Process completes and returns a result event (auth not blocked).
2. Personal config (memories, user rules) is absent from the `thread.started` event.

---

## How to run

```
cd playground
make pg-codex-simple-env
```

---

## Note

The `validate` function currently prints the full `thread.started` event since the field structure for Codex config metadata is not yet known. Update it once the event structure is observed in `poc-codex-pty`.
