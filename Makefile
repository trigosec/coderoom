BINARY        := coderoom
CODEX_VERSION := $(shell grep -v '^\#' CODEX_VERSION | tr -d '[:space:]')

.PHONY: build
build:
	go build -o bin/$(BINARY) ./cmd/coderoom

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: test
test:
	go test ./...

.PHONY: test-race
test-race:
	CGO_ENABLED=1 go test -race ./...

.PHONY: test-integration
test-integration:
	go test -tags integration ./...

# check-all runs everything. Track wall time — when it exceeds 15-20s
# consistently, split pre-commit to use `check` (lint + unit tests only).
.PHONY: check-all
check-all: lint test-race test-integration

.PHONY: check
check: lint test-race

.PHONY: upgrade-codex
upgrade-codex:
	./scripts/upgrade-codex.sh

.PHONY: install-hooks
install-hooks:
	ln -sf ../../scripts/pre-commit .git/hooks/pre-commit

# Playground targets
.PHONY: $(shell grep -h '\.PHONY' playground/Makefile | sed 's/\.PHONY: //')
%:
	$(MAKE) -C playground $@
