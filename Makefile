# KALUA Makefile

.PHONY: build test test-race fmt vet clean lint run serve lsp check new ai version gen-api check-api

# Build the KALUA binary
build:
	go build -o KALUA ./cmd/KALUA

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
ci: build test-race vet check-assets

# kalua.css is duplicated into the builder assets (the builder preview must load
# the runtime form stylesheet; embed patterns cannot reach outside the package
# dir, so a copy is embedded and served at /static/kalua.css).
sync-assets:
	cp internal/web/assets/kalua.css internal/builder/assets/kalua.css

check-assets:
	@cmp -s internal/web/assets/kalua.css internal/builder/assets/kalua.css && echo "kalua.css in sync" || (echo "ERROR: internal/builder/assets/kalua.css out of sync - run 'make sync-assets'" && exit 1)

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
	@echo "  gen-api      - Generate API reference (_opencode/skills/kalua-api/api.md)"
	@echo "  check-api    - Verify committed api.md matches generated output"

# Generate API reference markdown from api_doc.go
gen-api:
	go run ./cmd/kalua-apidoc -o _opencode/skills/kalua-api/api.md

# Check if committed api.md matches generated output (fails on drift)
check-api:
	go run ./cmd/kalua-apidoc -check