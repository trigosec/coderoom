# `.coderoom`

This directory contains repo-local participant and role configuration used by
Code Room.

Current contents:

- `participants/ada.yaml` defines the local `ada` participant
- `participants/tim.yaml` defines the local `tim` participant
- `prompts/roles/builder.md` defines the `builder` role prompt
- `prompts/roles/reviewer.md` defines the `reviewer` role prompt

Notes:

- role names used in `participants/*.yaml` must have a corresponding prompt in
  `prompts/roles/*.md`
- these prompts are intentionally short and repo-focused

Prompt provenance:

- the `builder` role prompt was written for this repository, using public agent
  prompt examples as inspiration
- notable references included Microsoft VS Code custom agent examples and the
  `agents-md` project
