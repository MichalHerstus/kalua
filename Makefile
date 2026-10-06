# KALUA Makefile

.PHONY: build test test-race fmt vet clean lint run serve lsp check new ai version gen-api check-api check-agents dist dist-verify dist-clean release-check

# ---- build metadata -------------------------------------------------------
# Stamped into the binary via -ldflags -X (see internal/version). All three
# come from git, so a build from a given commit reports that commit. The date
# is the *commit* date, not wall-clock, which keeps builds reproducible.
# Overridable for local experiments, e.g. `make dist GIT_VERSION=alfa`.
GIT_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GIT_COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
GIT_DATE    ?= $(shell git log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)

VERSION_PKG := kalua/internal/version
LDFLAGS     := -s -w \
	-X $(VERSION_PKG).Version=$(GIT_VERSION) \
	-X $(VERSION_PKG).Commit=$(GIT_COMMIT) \
	-X $(VERSION_PKG).Date=$(GIT_DATE)

# Build the KALUA binary
build:
	go build -trimpath -ldflags="$(LDFLAGS)" -o KALUA ./cmd/KALUA

# Run all tests
test:
	go test ./...

# Run tests with race detector
test-race:
	go test -race ./...

# Run tests without cache
test-clean:
	go clean -testcache && go test ./...

# Format code
fmt:
	gofmt -w ./internal ./cmd

# Check formatting
fmt-check:
	@gofmt -l ./internal ./cmd | grep -v "internal/common/outbox.go" | grep -v "internal/vm/app.go" && exit 1 || true

# Vet code
vet:
	go vet ./...

# Lint (fmt + vet)
lint: fmt-check vet

# Clean build artifacts
clean:
	rm -f KALUA
	rm -f *.vsix
	go clean -cache

# Run app in web mode
run:
	./KALUA run $(ARGS)

# Run app in serve mode
serve:
	./KALUA serve $(ARGS)

# Run LSP server
lsp:
	./KALUA lsp

# Check script syntax
check:
	./KALUA check $(ARGS)

# Scaffold new app
new:
	./KALUA new $(ARGS)

# AI generate app
ai:
	./KALUA ai $(ARGS)

# Print version
version:
	./KALUA version

# Build and run tests (CI pipeline)
ci: build test-race vet check-assets js-check

# ---- release artifacts ----------------------------------------------------
# Cross-compiled release binaries + SHA256SUMS, for the same platform set as
# .goreleaser.yml. Works without goreleaser installed; `make dist-verify`
# re-checks the checksums. Publishing is `make dist && gh release create <tag> dist/*`.
DIST_DIR       ?= dist
DIST_PLATFORMS ?= darwin/arm64 linux/amd64 windows/amd64

dist: dist-clean
	@mkdir -p $(DIST_DIR)
	@for t in $(DIST_PLATFORMS); do \
		os=$${t%/*}; arch=$${t#*/}; \
		case "$$os" in windows) ext=".exe" ;; *) ext="" ;; esac; \
		out="$(DIST_DIR)/KALUA_$${os}_$${arch}$$ext"; \
		echo "==> $$os/$$arch -> $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build -trimpath -ldflags="$(LDFLAGS)" -o "$$out" ./cmd/KALUA || exit 1; \
	done
	@cd $(DIST_DIR) && if command -v sha256sum >/dev/null 2>&1; \
		then sha256sum KALUA_* > SHA256SUMS; \
		else shasum -a 256 KALUA_* > SHA256SUMS; fi
	@$(MAKE) --no-print-directory dist-verify
	@echo "release artifacts in $(DIST_DIR)/ — version $(GIT_VERSION), commit $(GIT_COMMIT)"

# Re-verify $(DIST_DIR)/SHA256SUMS against the binaries.
dist-verify:
	@test -d $(DIST_DIR) || { echo "ERROR: $(DIST_DIR)/ not found - run 'make dist' first" && exit 1; }
	@cd $(DIST_DIR) && if command -v sha256sum >/dev/null 2>&1; \
		then sha256sum -c SHA256SUMS; \
		else shasum -a 256 -c SHA256SUMS; fi

dist-clean:
	@rm -rf $(DIST_DIR)

# Validate .goreleaser.yml without building (requires `goreleaser`).
release-check:
	@goreleaser check

# kalua.css is duplicated into the builder assets (the builder preview must load
# the runtime form stylesheet; embed patterns cannot reach outside the package
# dir, so a copy is embedded and served at /static/kalua.css).
sync-assets:
	cp internal/web/assets/kalua.css internal/builder/assets/kalua.css

check-assets:
	@cmp -s internal/web/assets/kalua.css internal/builder/assets/kalua.css && echo "kalua.css in sync" || (echo "ERROR: internal/builder/assets/kalua.css out of sync - run 'make sync-assets'" && exit 1)

# JS asset syntax check (node must be on PATH). Guards the embedded browser
# clients so a broken builder.js / app.js fails CI instead of shipping a dead UI.
js-check:
	@for f in internal/builder/assets/builder.js internal/builder/assets/markdown.js internal/web/assets/app.js internal/cli/wasm_assets/app.minimal.js; do \
		node --check "$$f" >/dev/null 2>&1 || (echo "ERROR: $$f failed node --check" && exit 1); \
	done
	@echo "JS assets parse clean"

# Build VSCode extension
ext-build:
	cd extensions/vscode-kalua && npm install --no-audit --no-fund --cache /tmp/kalua-npm-cache && npm run compile && npm run package

# Install VSCode extension
ext-install:
	code --install-extension extensions/vscode-kalua/kalua.vsix --force

# Show help
help:
	@echo "KALUA Makefile targets:"
	@echo "  build        - Build KALUA binary"
	@echo "  test         - Run all tests"
	@echo "  test-race    - Run tests with race detector"
	@echo "  fmt          - Format code with gofmt"
	@echo "  fmt-check    - Check formatting (excludes pre-existing files)"
	@echo "  vet          - Run go vet"
	@echo "  lint         - Run fmt-check + vet"
	@echo "  clean        - Remove build artifacts"
	@echo "  run ARGS=... - Run app in web mode (e.g., make run ARGS='app.lua --port 8080')"
	@echo "  serve ARGS=..- Run app in serve mode (e.g., make serve ARGS='app.lua --port 8080')"
	@echo "  lsp          - Start LSP server over stdio"
	@echo "  check ARGS=..- Check script syntax"
	@echo "  new ARGS=... - Scaffold new app"
	@echo "  ai ARGS=...  - AI builder (generate, fix, validate)"
	@echo "  version      - Print version"
	@echo "  ci           - Full CI pipeline (build + test-race + vet)"
	@echo "  ext-build    - Build VSCode extension"
	@echo "  ext-install  - Install VSCode extension"
	@echo "  gen-api      - Generate API reference (_opencode/skills/kalua-api/api.md) + quickref (docs/agentic/quickref.md)"
	@echo "  check-api    - Verify committed api.md + quickref.md match generated output"
	@echo "  dist         - Cross-build release binaries + SHA256SUMS into dist/ (stamped from git)"
	@echo "  dist-verify  - Re-verify dist/SHA256SUMS against the binaries"
	@echo "  dist-clean   - Remove dist/"
	@echo "  release-check - Validate .goreleaser.yml (needs goreleaser)"

# Generate API reference markdown from api_doc.go, plus the compact quickref
# card used by agents (docs/agentic/quickref.md).
gen-api:
	go run ./cmd/kalua-apidoc -o _opencode/skills/kalua-api/api.md -quickref docs/agentic/quickref.md

# Check if committed api.md + quickref.md match generated output (fails on drift)
check-api:
	go run ./cmd/kalua-apidoc -check

# Verify agentic development artifacts exist and are wired correctly
check-agents:
	@test -f docs/agentic/development.md && echo "development.md exists" || (echo "ERROR: docs/agentic/development.md missing" && exit 1)
	@test -f docs/agentic/quickref.md && echo "quickref.md exists" || (echo "ERROR: docs/agentic/quickref.md missing" && exit 1)
	@test -f CLAUDE.md && echo "CLAUDE.md exists" || (echo "ERROR: CLAUDE.md missing" && exit 1)
	@test -f .github/copilot-instructions.md && echo "copilot-instructions.md exists" || (echo "ERROR: .github/copilot-instructions.md missing" && exit 1)
	@test -f .cursor/rules/kalua.mdc && echo "cursor rules exist" || (echo "ERROR: .cursor/rules/kalua.mdc missing" && exit 1)
	@test -f AGENTS.md && echo "AGENTS.md exists" || (echo "ERROR: AGENTS.md missing" && exit 1)
	@$(MAKE) check-api
	@echo "All agentic artifacts verified"