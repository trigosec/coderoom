# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Read First

- Product concept and agent model: `docs/design/concept.md`
- System architecture: `docs/design/architecture.md`
- Coding and review style (authoritative): `docs/code-review.md`

## Commands

```
make build              # build ./cmd/coderoom
make lint               # run golangci-lint
make test               # unit tests
make test-integration   # integration tests (requires Codex installed)
make check              # lint + test
make check-all          # lint + test + test-integration (used by pre-commit)
make upgrade-codex      # advance pinned Codex version to latest
make install-hooks      # install pre-commit hook
```

Playground experiments are in `playground/` with their own Makefile. Run from the repo root: `make pg-codex-stdio`, etc.
