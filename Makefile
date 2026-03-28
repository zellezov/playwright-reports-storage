.PHONY: build run test test-integration lint clean coverage

BIN     := prs
COV_DIR := coverdata

## build: compile the binary
build:
	go build -o $(BIN) ./cmd/prs

## run: build and start the server (uses defaults; override with env vars)
run: build
	./$(BIN)

## test: run all tests
test:
	go test ./...

## test-integration: run integration tests with verbose output
test-integration:
	go test ./test/integration/... -v -timeout 60s

## lint: run go vet (install staticcheck separately for more thorough linting)
lint:
	go vet ./...

## coverage: run unit + integration tests and produce a combined coverage report
## Unit tests run normally with -cover; integration tests run against a
## coverage-instrumented binary (go build -cover, available since Go 1.20).
coverage:
	@mkdir -p $(COV_DIR)
	@echo "--- unit tests ---"
	go test -coverprofile=$(COV_DIR)/unit.out ./internal/... ./cmd/...
	@echo "--- integration tests (instrumented binary) ---"
	go build -cover -o $(BIN)-instrumented ./cmd/prs
	GOCOVERDIR=$(COV_DIR) ./$(BIN)-instrumented &
	sleep 1
	go test ./test/integration/... -v -timeout 60s; \
		kill $$(lsof -ti :3912) 2>/dev/null || true
	go tool covdata textfmt -i=$(COV_DIR) -o=$(COV_DIR)/integration.out
	@echo "--- combined coverage ---"
	go tool cover -func=$(COV_DIR)/unit.out
	@echo ""
	@echo "Integration coverage written to $(COV_DIR)/integration.out"
	@echo "Run: go tool cover -html=$(COV_DIR)/unit.out   (unit)"
	@echo "Run: go tool cover -html=$(COV_DIR)/integration.out   (integration)"

## clean: remove build artifacts
clean:
	rm -f $(BIN) $(BIN)-instrumented
	rm -rf $(COV_DIR)
