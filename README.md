# Code Room

A CLI-native workspace for coordinating AI coding agents.

Code Room brings mob programming to the terminal: a small team of specialised agents (builder, architect, reviewer, security) working together in a shared room, with git as the common workspace and you as the decision authority.

---

## The Problem

Mob programming is a high-bandwidth way to work. The whole team focuses on one task, sharing ideas, challenging assumptions, and converging on better solutions for hard problems.

AI agents are powerful, but without a mob-style workflow the work can fragment across tools and threads. It becomes harder to keep a shared understanding, avoid duplicated effort, and review the changes with confidence.

Code Room aims to replicate the mob programming experience in the terminal, with named roles, a shared room for coordination, and a shared git workspace where the human remains the decision authority.

---

## Status (Shared Room Alpha)

This repository is early-stage:

- The UI is a **single shared room** (no private agent tabs yet).
- The focus is on **comfortable single-agent use**.
- The current backend is **Codex app-server** (driven over JSON-RPC via stdio).

Roadmap: see `docs/roadmap.md`.

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

`ada` is the alias you will use to address this agent in the room. For now,
the backend (Codex) and role are implicit; a future version will let you
specify both.

Send a message:

```text
@ada implement a small change: ...
```

---

## Using Code Room (today)

Useful commands:

```text
/who            # show roster
/help           # show commands
/cancel <alias> # interrupt current work (best-effort)
/remove <alias> # stop + remove agent
/quit           # exit
```

If only one agent is present, plain text is broadcast to it (equivalent to
sending to that agent).

---

## Design docs

- `docs/design/concept.md`
- `docs/design/architecture.md`

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
