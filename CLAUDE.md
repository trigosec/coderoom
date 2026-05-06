# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Code Room is a CLI-native multi-agent coding workspace, a coordination layer that lets developers invite multiple AI coding tools (Claude Code, Codex, Aider) into a shared session with distinct roles, controlled autonomy, and structured communication channels.

The project is in early stage: architecture is defined, implementation is starting.

## Core Concepts

**Agent model**: each agent has a backend (CLI tool), alias (e.g. `ada`), role (builder/reviewer/tester/architect/security-reviewer), capability set (read/write/test/shell), and initiative level (manual/suggestive/active/autonomous).

**Git as shared workspace**: all agents share a single git worktree. `git diff` is the shared language of change. Code Room does not wrap git; agents interact with it directly through their CLI tools.

**Communication channels**:
- *Shared room*: visible to all agents and the user; primary coordination space
- *Private channel*: user-to-one-agent; includes reasoning and approval flows; not visible to others
- *System*: internal session events

**Session state** is persisted under `.coderoom/<room-name>/` with `session.json`, `events.jsonl` (append-only event log), `decisions.md`, and per-agent context under `agents/`.

## Architecture

The **Session Controller** is the central orchestrator. All commands, messages, and state changes route through it.

Key components:
- **Agent Registry**: tracks alias, backend, role, capabilities, initiative, and status (`running`/`paused`/`crashed`) for each agent
- **Agent Runtime**: manages CLI processes (start/stop/restart, I/O, crash detection); agents are managed processes, not API clients
- **Backend Adapters**: thin wrappers per CLI tool exposing `start()`, `send_message()`, `read_output()`, `stop()`; supported backends are Claude Code, Codex, Aider
- **Sandbox Controller**: OS-level constraints per agent (chroot/filesystem scope, network egress policy) based on role and capabilities
- **Message Router**: routes across `shared`, `private`, and `system` channels; no agent-to-agent private messaging
- **Policy Engine**: session-level enforcement of file write permissions, initiative behaviour, and risk boundaries

The system is event-driven (`SessionCreated`, `AgentInvited`, `MessageSent`, `FileChanged`, etc.) with events appended to `events.jsonl` for replay and debugging.

## Design Principles

1. Human-in-the-loop authority; human is the final decision-maker
2. Agents are collaborators, not owners
3. Shared room is the coordination layer
4. Git is the source of truth
5. Explicit before autonomous; start at manual initiative, grant autonomy gradually
6. Transparency over hidden behaviour
7. Structure over free-form chat
