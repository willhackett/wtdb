MODULE  := github.com/willhackett/wtdb
BIN     := wtdb
OUT     := bin/$(BIN)

# Install locations (override: make install PREFIX=/usr/local)
PREFIX  ?= $(HOME)/.local
BINDIR  := $(PREFIX)/bin
MANDIR  := $(PREFIX)/share/man/man1

# Version stamped into the binary; uses the latest tag, else the short SHA.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(MODULE)/internal/cli.Version=$(VERSION)

.DEFAULT_GOAL := help

## help: show this help
.PHONY: help
help:
	@echo "wtdb — make targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
	@echo
	@echo "Version: $(VERSION)   Install prefix: $(PREFIX)"

## build: compile the binary to ./bin/wtdb
.PHONY: build
build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(OUT) .

## install: build and install the binary + man page under PREFIX
.PHONY: install
install: build
	@mkdir -p $(BINDIR) $(MANDIR)
	install -m 0755 $(OUT) $(BINDIR)/$(BIN)
	install -m 0644 man/$(BIN).1 $(MANDIR)/$(BIN).1
	@echo "Installed $(BINDIR)/$(BIN) and $(MANDIR)/$(BIN).1"
	@command -v $(BIN) >/dev/null 2>&1 || echo "note: $(BINDIR) is not on your PATH"

## uninstall: remove the installed binary and man page
.PHONY: uninstall
uninstall:
	rm -f $(BINDIR)/$(BIN) $(MANDIR)/$(BIN).1
	@echo "Removed $(BINDIR)/$(BIN) and $(MANDIR)/$(BIN).1"

## test: run the full test suite (Postgres integration tests included if a server is reachable)
.PHONY: test
test:
	go test ./...

## test-short: run unit tests only, skipping Postgres integration tests
.PHONY: test-short
test-short:
	go test -short ./...

## cover: run tests with a coverage summary
.PHONY: cover
cover:
	go test -cover ./...

## vet: run go vet
.PHONY: vet
vet:
	go vet ./...

## fmt: format all Go source
.PHONY: fmt
fmt:
	gofmt -w .

## lint: check formatting and run go vet
.PHONY: lint
lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...

## tidy: tidy go.mod / go.sum
.PHONY: tidy
tidy:
	go mod tidy

## man: preview the man page
.PHONY: man
man:
	man ./man/$(BIN).1

## check: format check, vet, and tests — what CI runs
.PHONY: check
check: lint test

## release-check: validate the GoReleaser config (requires goreleaser)
.PHONY: release-check
release-check:
	goreleaser check

## snapshot: build a local release snapshot without publishing (requires goreleaser)
.PHONY: snapshot
snapshot:
	goreleaser release --snapshot --clean

## clean: remove build artifacts
.PHONY: clean
clean:
	rm -rf bin dist
