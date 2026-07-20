# Participant Setup

This document describes how repo-local participant and role configuration works
in coderoom.

For design rationale and runtime integration details, see
`docs/design/participant-roles.md`.

## Overview

coderoom separates two concerns:

- a **participant** is the named collaborator you invite into the room
- a **role** is the behavioral prompt attached to that participant

Participant definitions are stored in YAML. Role prompts are stored in
Markdown. Both live under the repo-local `.coderoom/` directory.

## File Layout

```text
.coderoom/
  participants/
    ada.yaml
    tim.yaml
  prompts/
    roles/
      builder.md
      reviewer.md
```

- `participants/*.yaml` defines participant identity and role selection
- `prompts/roles/*.md` defines the corresponding role behavior

Role names used in `participants/*.yaml` must have a matching prompt file in
`prompts/roles/`.

## Participant Definitions

Each participant is defined by one YAML file:

```yaml
alias: ada
role: builder
```

Supported fields:

- `alias`
- `role`

Rules:

- the filename stem must match `alias`
- `alias` must not be empty
- `role` is optional
- if `role` is set, `.coderoom/prompts/roles/<role>.md` must exist

If `role` is omitted, empty, or `null`, the participant starts without a role
prompt.

## Role Prompts

Role prompts are plain Markdown files. Keep them short, concrete, and focused
on the behavior you want in this repository.

Example:

```md
You are a reviewer.

Review changes critically for correctness, regressions, and missing coverage.
Prioritize real defects and risky assumptions over style feedback.
```

The prompt file is the source of truth for what that role means in the repo.

## Invite Behavior

When you run:

```text
/invite ada
```

coderoom does the following:

1. Checks for `.coderoom/participants/ada.yaml`.
2. If the file exists, validates it.
3. If the file defines a role, loads `.coderoom/prompts/roles/<role>.md`.
4. Synthesizes the participant startup prompt from identity plus role prompt.
5. Starts the agent.

If `.coderoom/participants/ada.yaml` does not exist, invite still succeeds with
identification-only setup.

## Validation Failures

Invite should fail clearly when:

- the YAML is malformed
- `alias` is empty
- the filename does not match `alias`
- the configured role prompt file is missing

These failures should be fixed in repo configuration rather than worked around
at runtime.

## Current Repository Example

This repository currently defines:

- `.coderoom/participants/ada.yaml` with role `builder`
- `.coderoom/participants/tim.yaml` with role `reviewer`
- `.coderoom/prompts/roles/builder.md`
- `.coderoom/prompts/roles/reviewer.md`
