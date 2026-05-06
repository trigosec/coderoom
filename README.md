# Code Room

A CLI-native multi-agent coding workspace.

Code Room lets you invite multiple AI coding tools into a shared session, assign them roles, control their autonomy, and coordinate their collaboration through structured communication channels.

Rather than being another coding agent, Code Room acts as a coordination layer for multiple agents working together in a controlled environment.

---

## The Problem

A single LLM does not consistently produce the quality needed for production code. Different models have different blind spots. A reviewer that did not write the code catches things the author missed — the same reason human code review works.

Working with LLMs well requires structure: clear roles, controlled autonomy, and a shared workspace where the human remains the decision authority.

Code Room formalises that structure.

---

## How It Works

A session consists of multiple AI agents, each with:

- **A backend**: the underlying CLI tool (Claude Code, Codex, Aider, etc.)
- **An alias**: a human-friendly name for interaction (e.g. `ada`, `turing`)
- **A role**: a behavioral contract (builder, reviewer, tester, architect)
- **Capabilities**: what the agent is allowed to do (read, write, test, shell)
- **An initiative level**: how autonomously the agent acts

Agents share a git worktree. The repo is the shared workspace. `git diff` is the shared language of change.

---

## Example Session

```
/create session bugfix-auth
/invite claude-code as ada role builder
/invite codex as turing role reviewer

/task "Fix failing OAuth refresh test"

@ada implement the fix
@turing review the diff
@hopper run tests

/commit "Fix OAuth refresh test"
```

---

## Roles

| Role | Responsibility |
|---|---|
| Builder | Writes and modifies code |
| Reviewer | Critiques changes, identifies issues |
| Tester | Runs tests, validates behaviour |
| Architect | Proposes system design decisions |
| Security Reviewer | Evaluates risks and vulnerabilities |

---

## Initiative Levels

| Level | Behaviour |
|---|---|
| Manual | Only acts when explicitly addressed |
| Suggestive | May comment, cannot change state |
| Active | May propose changes |
| Autonomous | May act within policy constraints |

Start explicit. Grant autonomy gradually.

---

## Design Principles

1. Human-in-the-loop authority
2. Agents are collaborators, not owners
3. Shared room is the coordination layer
4. Git is the source of truth
5. Explicit before autonomous
6. Transparency over hidden behaviour

---

## Status

Early stage. Architecture is defined. Implementation is starting.

Contributions, issues, and feedback welcome.

---

## Motivation

This project grew out of a personal workflow: using two LLMs in structured roles (one implements, one reviews) produces materially better code than using one. Code Room is the attempt to formalise and automate that pattern.

More context: [Quality vs Vibes: what disciplined AI-assisted development looks like](#)
