# Code Room

A programmable multi-agent coding room.

Code Room brings mob programming to the terminal. You and a team of AI coding
agents work together in a shared room, with git as the common workspace and you
as the decision authority.

Start by collaborating step by step. When a coordination pattern becomes
repetitive, automate it with the same conversational primitives using Code
Room's prompt-based programming language.

> Collaborate first. Automate when ready.

---

## The Problem

Mob programming is a high-bandwidth way to work. The whole team focuses on one task, sharing ideas, challenging assumptions, and converging on better solutions for hard problems.

AI agents are powerful, but without a mob-style workflow the work can fragment
across tools and threads. It becomes harder to maintain shared understanding,
avoid duplicated effort, and review changes with confidence. Workflow
automation often creates the opposite problem: it hides the work behind a
fixed pipeline and removes the human from the collaboration.

Code Room combines both modes. Developers can work interactively with named
agents in a shared terminal room, hand work between them, or compose bounded
workflows from general language primitives. The human chooses how much control
to keep at each step and can move between collaboration and automation without
leaving the room.

---

## How it works

Code Room has four layers that build on one another:

1. **Mob programming:** the developer and agents work on the same problem in a
   shared room and git workspace.
2. **Multi-agent collaboration:** named participants can be addressed directly
   and work can be handed between them.
3. **A prompt-based programming language:** room interactions and deterministic
   checks become composable commands.
4. **Progressive automation:** the developer decides whether to interact one
   step at a time or let a bounded workflow run.

For example, the same session can move from an ordinary participant message:

```text
@ada investigate the failing tests
```

to coordination between agents:

```text
/handoff ada turing
```

to an automated, bounded workflow:

```text
/def tests /shell go test ./...
/loop @ada make the tests pass without weakening them /until /tests /max 3
```

The loop asks Ada to work, evaluates `/tests` after each turn, and continues
with the latest failure evidence until the command succeeds or three agent
turns have completed. This behavior is composed from general primitives; it is
not a built-in "fix tests" workflow.

---

## Built with Codex

Code Room was developed by dogfooding the collaboration model it provides. The
developer worked with named Codex agents in the shared repository: **Ada** acted
as the builder and **Tim** as the reviewer. Git diffs, focused handoffs, and
small commits formed the coordination loop.

Codex, powered by GPT-5.6, accelerated the implementation by exploring the
existing architecture, extracting the prompt-language parser, implementing
shell execution and loop state transitions, writing tests and documentation,
and turning the implementation plan into small GitHub issues. The reviewer
agent repeatedly found lifecycle and presentation problems, such as shell
processes surviving shutdown, condition timing, and command output becoming
hidden in the room preview, which were addressed before moving to the next
slice.

The developer made the key product and language decisions. These included
rejecting workflow-specific commands such as `/review`, choosing general
primitives instead, removing unnecessary quoting, parsing `/loop` control
clauses from the end of the line, separating `/def` from invocation, and
correcting the loop from a preconditioned `while` model to `do...until`
semantics. The developer also chose the progressive-automation positioning and
the split between compact human-facing shell previews and complete condition
evidence sent to agents.

The final result reflects that division of work: GPT-5.6 and Codex shortened
the distance from design discussion to tested implementation, while the human
set the product direction, resolved semantic ambiguities, and decided what
belonged in the language.

---

## Status (Prompt Language Version 0)

This repository is early-stage:

- The UI is a **single shared room** for the developer and named agents.
- The current backend is **Codex app-server** (driven over JSON-RPC via stdio).
- The prompt language supports direct messages, broadcasts, handoffs, shell
  commands, shell-backed command definitions, and bounded loops.
- Definitions are scoped to the running room and accept no parameters.
- Loop bodies contain one participant prompt and one shell-backed completion
  condition. Nested or concurrent loops are not supported.

---

## Install

### Download a release

Prebuilt archives are published on the [GitHub Releases](https://github.com/trigosec/coderoom/releases/latest) page.

Choose the archive that matches your platform:

- `coderoom_<version>_darwin_arm64.tar.gz`
- `coderoom_<version>_darwin_amd64.tar.gz`
- `coderoom_<version>_linux_arm64.tar.gz`
- `coderoom_<version>_linux_amd64.tar.gz`

Extract the archive and run the binary:

```bash
tar -xzf coderoom_<version>_<os>_<arch>.tar.gz
./coderoom
```

Checksums are published with each release as `checksums.txt`.

### Build from source

Prerequisites:

- Go (see `go.mod`)
- Node.js + `npx`
- A working Codex CLI setup (sanity check: `npx @openai/codex app-server` should start)

Build and run:

```bash
make build
./bin/coderoom
```

---

## Quick start

Start one agent:

```text
/invite ada
```

`ada` is the alias you will use to address this agent in the room.

When `.coderoom/participants/ada.yaml` exists, Code Room loads that
participant definition and any referenced role prompt before starting the
agent. If no participant definition exists, invite still works with
identification-only setup.

Send a message:

```text
@ada implement a small change: ...
```

### Participant setup

Participant and role configuration lives under `.coderoom/`:

```text
.coderoom/
  participants/
    ada.yaml
  prompts/
    roles/
      builder.md
```

Minimal participant definition:

```yaml
alias: ada
role: builder
```

Minimal role prompt:

```md
You are a builder.

Implement the requested change directly in the codebase.
Follow existing code patterns and keep edits minimal, focused, and practical.
```

See `docs/participants.md` for the full setup and validation rules.

## Commands

Useful commands:

```text
/invite <alias>                         # start an agent
@<alias> <prompt>                       # send to one agent
<prompt>                                # broadcast to all agents
/handoff <from> <to>                    # transfer latest output
/shell <program>                        # execute a shell program
/def <name> /shell <program>            # define a reusable command
/<name>                                 # invoke a defined command
/loop @<alias> <prompt> /until /<name> /max <turns>
/who                                    # show roster
/cancel <alias>                         # interrupt current work (best-effort)
/remove <alias>                         # stop and remove an agent
/help                                   # show commands
/quit                                   # exit
```

If only one agent is present, plain text is broadcast to it (equivalent to
sending to that agent).

---

## Design docs

- `docs/design/concept.md`
- `docs/design/architecture.md`
- `docs/design/participant-roles.md`
- `docs/design/prompt-language.md`

---

## Development

```
make test  # quick test suite
make pre-commit # golangci-lint and test suite with -race
make test-all   # full test suite including integration tests
```

Integration tests (require external CLIs):

```bash
make test-integration
```
