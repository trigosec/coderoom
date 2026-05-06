# Code Room: Agent Instructions

This repo is **Code Room**: a CLI-native multi-agent coding workspace. Agents collaborate through a shared git worktree; `git diff` is the shared language of change.

## Read First (design + expectations)

- Product concept: `docs/design/concept.md`
- System architecture: `docs/design/architecture.md`
- Coding + review style guide (authoritative): `docs/code-review.md`

## Experiments / Playground

We keep research notes in `docs/playground/` and runnable prototypes in `playground/`.

- Synthesis doc: `docs/playground/research-llm-cli.md`
- Run a POC: `cd playground && make pg-…` (see `playground/Makefile`)

Key findings to keep in mind when implementing:
- **Claude Code** and **Codex CLI** often require a **PTY** when driven as subprocesses (their normal CLI modes assume a TTY).
- For Codex, prefer `app-server` over stdio for a persistent managed-process integration (see `docs/playground/poc-codex-stdio.md`).

## Code Style (follow `docs/code-review.md`)

Highlights:
- Keep functions roughly one screen; top-level functions should read like algorithms via named helpers.
- Naming must signal side effects: `Get*/Read*/Parse*` are pure-by-contract; `Create*/Update*/Start*` imply effects.
- Prefer early returns; keep error handling local (`if err != nil { return err }`).
- Tests: use table-driven tests for input variants; don’t assert on error strings; prefer fakes over mocks.
- Tag integration tests with `//go:build integration` and run them separately (they require external CLIs).

## Working Agreement

- Make minimal, focused changes aligned with the design docs.
- Don’t auto-commit; leave staging/commits to the human unless explicitly asked.
- If a change impacts architecture/UX, update the relevant doc under `docs/` alongside the code.

