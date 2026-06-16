# XEME OS — Makefile
# Common dev tasks. Run `make` to see help.

GO        ?= go
BIN_DIR   ?= bin
CMDS      := xeme xeme-os xeme-mcp xeme-campaigns xeme-ledger-server xeme-workflows

# Map cmd name → directory under cmd/
CMD_DIR_xeme                  := xeme
CMD_DIR_xeme-os               := xeme-os
CMD_DIR_xeme-mcp              := xeme-mcp
CMD_DIR_xeme-campaigns        := xeme-campaigns
CMD_DIR_xeme-ledger-server    := xeme-ledger
CMD_DIR_xeme-workflows        := xeme-workflows

.PHONY: help
help: ## show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## build all binaries into ./bin/
	@mkdir -p $(BIN_DIR)
	@for cmd in $(CMDS); do \
		dir=$$(echo "CMD_DIR_$$cmd" | xargs -I{} bash -c "echo \$${}"); \
		echo "→ building $$cmd (./cmd/$$dir)"; \
		$(GO) build -trimpath -o $(BIN_DIR)/$$cmd ./cmd/$$dir; \
	done
	@echo "✓ all built into $(BIN_DIR)/"
	@ls -lh $(BIN_DIR)/

.PHONY: build-local
build-local: ## build all binaries into repo root (default layout)
	@for cmd in $(CMDS); do \
		dir=$$(echo "CMD_DIR_$$cmd" | xargs -I{} bash -c "echo \$${}"); \
		echo "→ building $$cmd"; \
		$(GO) build -o $$cmd ./cmd/$$dir; \
	done

.PHONY: test
test: ## run all unit tests
	$(GO) test -v -count=1 ./... | tail -100

.PHONY: test-short
test-short: ## run tests without verbose
	$(GO) test -count=1 ./...

.PHONY: vet
vet: ## run go vet
	$(GO) vet ./...

.PHONY: fmt
fmt: ## format all Go files
	$(GO) fmt ./...

.PHONY: tidy
tidy: ## go mod tidy
	$(GO) mod tidy

.PHONY: clean
clean: ## remove built binaries
	rm -f $(CMDS) $(addprefix $(BIN_DIR)/,$(CMDS))
	rm -rf dist/

.PHONY: run-dashboard
run-dashboard: build-local ## build + run the local dashboard
	./xeme-os --host 127.0.0.1 --port 4903

.PHONY: run-mcp
run-mcp: build-local ## build + run the MCP server (stdio)
	./xeme-mcp

.PHONY: install-deps
install-deps: ## install Go 1.23+ via brew (macOS)
	brew install go

.PHONY: tag
tag: ## cut a new release tag (usage: make tag V=1.3.1)
	@test -n "$(V)" || (echo "Usage: make tag V=1.3.1" && exit 1)
	git tag -a v$(V) -m "Release v$(V)"
	git push origin v$(V)
	@echo "✓ Tagged v$(V). GitHub Actions will build and publish the release."
