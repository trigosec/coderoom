# Code Room: Architecture

## Overview

A Code Room session consists of multiple AI agents sharing a git worktree. The repo is the shared workspace. `git diff` is the shared language of change.

The Session Controller is the central orchestrator. All commands, messages, and state changes flow through it.

---

## Component Map

```
+-----------------------------+
|         Terminal UI          |
| shared / private views       |
+--------------+--------------+
               |
               v
+-----------------------------+
|       Session Controller     |
| commands, routing, policy    |
+--------------+--------------+
               |
       +-------+-------+
       |               |
       v               v
+-------------+   +----------------+
| Agent       |   | Sandbox        |
| Registry    |   | Controller     |
+-------------+   +----------------+
       |               |
       v               v
+-------------+   +----------------+
| Agent       |   | chroot / net   |
| Runtime     |   | policy         |
+-------------+   +----------------+
       |
       v
+-------------+
| CLI Backend |
+-------------+
```

---

## Components

### 1. Terminal UI

- Shared room view (all agents + user)
- Private agent tabs
- Session roster with status indicators
- Command input

The UI is intentionally lean. It projects session and participant state; it does
not talk to agent processes directly. All workflow invariants are enforced below
the UI layer.

---

### 2. Session Controller

Central orchestrator. Responsible for:

- Parsing commands
- Routing messages to agents or channels
- Enforcing policies (capabilities, initiative levels)
- Tracking session state
- Controlling agent lifecycle

The session controller is the sole mutator of participant runtime state. It
coordinates concurrency, owns reader goroutines, and prevents invalid
transitions (for example, sends while a turn is already in flight).

---

### 3. Participant Registry

Tracks all participants in the session:

```
alias, backend, role, capabilities, initiative, status
```

Status values: `idle`, `starting`, `preparing`, `working`, `crashed`

The participant is the stateful runtime entity. Beyond identity and coarse
status, it carries active-turn runtime state used by the session and UI (for
example, tracked open streams while the participant is working) and enforces
runtime invariants for legal transitions.

---

### 4. Agent Runtime

Manages CLI processes:

- Start and stop
- Send input to the process
- Capture and parse output
- Detect crashes and restart if needed

Agents are treated as managed processes, not API clients.
They are transport adapters with minimal internal state beyond buffering and
protocol handling.

---

### 5. Backend Adapters

Thin wrappers for each supported CLI tool. Interface:

```
start()
send_message(text)
read_output() -> text
stop()
```

Backends are treated as black boxes. Adapters handle tool-specific I/O quirks. Supported backends: Claude Code, Codex, Aider.

An adapter is intentionally stateless at the workflow level: it does not own
session or participant semantics. Runtime meaning is added one layer up by the
participant and session controller.

---

### 6. Sandbox Controller

Each agent runs inside a sandbox that constrains what it can access at the OS level. Code Room does not attempt to intercept decisions made inside the CLI tool itself. Instead, it defines the boundary within which the CLI operates.

Sandbox constraints:

- **Filesystem**: chroot or equivalent, scoped to the worktree and permitted paths
- **Network**: outbound connections restricted by policy (e.g. allow registry/CDN, deny arbitrary egress)

The sandbox is configured per agent at launch, based on the agent's role and capabilities.

Agent actions and approval requests are handled in the private channel between the human and that agent. The shared room shows only a flag when an agent requires attention:

```
[hopper requires attention]
```

The human reviews and responds in the private tab. Other agents are not exposed to the operational detail.

---

### 7. Message Router

Routes messages across channels:

- `shared`: broadcast to all agents and user
- `private`: user to one agent (includes reasoning and approval flows)
- `system`: internal session events

Rules:
- Shared room is the primary coordination layer
- No hidden agent-to-agent channels
- Reasoning and approval requests stay in private channels
- Shared room shows attention flags, not operational detail

---

### 8. Policy Engine

Controls what each agent is permitted to do at the session level:

- File write permissions (based on role)
- Initiative behaviour enforcement
- Risk boundary checks

Filesystem and network constraints are enforced by the Sandbox Controller at the OS level, not by the Policy Engine intercepting CLI behaviour.

---

## Execution Model

### Shared Worktree

All agents operate on the same git worktree. There is no patch synchronisation problem: `git diff` gives every agent and the user a consistent view of current changes. Agents interact with git directly through their CLI tools. Code Room does not wrap or duplicate git commands.

### Direct File Editing

Agents modify files directly, subject to sandbox constraints. The human retains control through git: staging, reverting, and committing happen outside the room in a normal terminal.

---

## Event Model

The system operates via discrete events. This enables replay, debugging, and session persistence.

```
SessionCreated
AgentInvited
AgentStarted
MessageSent
CommandIssued
FileChanged
AgentCrashed
AgentRestarted
DecisionRecorded
```

Events are appended to `events.jsonl` in the session directory.

---

## Session State

Session state is persisted under `.coderoom/room-name/`, allowing multiple rooms per repo and rooms that reference paths outside a single repo.

```
.coderoom/
  <room-name>/
    session.json      # agents, roles, permissions, current task
    events.jsonl      # append-only event log
    decisions.md      # human-approved decisions
    agents/           # per-agent context and scratchpad
```

State includes: agents, roles, permissions, messages, decisions, and a reference to the current repo state.

---

## Runtime Flow

```
User: /task "Fix OAuth refresh test"

ada (builder):     proposes plan
turing (reviewer): critiques the approach
User: @ada proceed
ada:               implements fix (modifies repo)
User: @turing review diff
turing:            reviews using git diff
[hopper requires attention]
User: (private tab) approves test run
hopper:            executes tests, reports result to shared room
```

---

## System Model

```
agents      = managed processes
roles       = behavioral contracts
aliases     = interaction identity
messages    = routed events
git         = shared state
diff        = change context
sandbox     = OS-level boundary
controller  = session authority
participant = stateful collaborator
agent       = stateless transport adapter
human       = final decision-maker
```

---

## Open Questions

- Agent lifecycle: pause/resume without losing context
- Autonomy escalation: criteria for granting higher initiative
- Multi-session or distributed setups
- Sandbox implementation: chroot vs container vs namespace isolation
- Network policy granularity: per-agent or per-role
