# POC: Suppressing personal config with CLAUDE_CODE_SIMPLE=1

## Goal

Validate that setting `CLAUDE_CODE_SIMPLE=1` suppresses personal config (plugins, MCP servers, skills) when spawning Claude Code as a subprocess, without blocking authentication.

---

## Background

In the first POC (`poc-claude-pty.md`), running without `--bare` caused Claude to load the full personal config: MCP servers, plugins, memory paths, and skills were all present in the `system init` event. `--bare` suppressed these but also blocked authentication.

`CLAUDE_CODE_SIMPLE=1` is an environment variable set internally by `--bare`. The question was whether it could be set independently to suppress config loading without triggering the auth failure.

---

## Approach

Spawn Claude with `CLAUDE_CODE_SIMPLE=1` in the environment. Inspect the `system init` event and check whether `plugins`, `mcp_servers`, and `skills` are empty. Confirm the process completes successfully.

---

## Findings

**Failed.** `CLAUDE_CODE_SIMPLE=1` blocks authentication in the same way `--bare` does. The process returns "Not logged in · Please run /login" and exits without producing a `result` event.

`CLAUDE_CODE_SIMPLE=1` is not an independent knob; it is exactly what `--bare` sets. There is no known flag or environment variable that suppresses personal config loading without also breaking auth.

---

## Conclusion

Personal config loading cannot be suppressed without breaking auth. The practical path forward is to accept it:

- Plugins, MCP servers, and skills load with each agent process.
- MCP servers appear as `needs-auth` in the `system init` event and are not active.
- The `system init` event is informational; the runtime can ignore these fields.
- Real agent isolation comes from the sandbox controller (filesystem, network), not from config suppression.

---

## How to run

```
cd playground
make pg-claude-simple-env
```
