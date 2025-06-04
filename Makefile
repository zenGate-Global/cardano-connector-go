# Determine root directory
ROOT_DIR=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

# Gather all .go files for use in dependencies below
GO_FILES=$(shell find $(ROOT_DIR) -name '*.go')

# Gather list of expected binaries
BINARIES=$(shell cd $(ROOT_DIR)/cmd && ls -1 | grep -v ^common)

# Load .env file if it exists
-include .env

.PHONY: mod-tidy test test-match format lint clean

mod-tidy:
	# Needed to fetch new dependencies and add them to go.mod
	@go mod tidy

test:
	@echo "Running tests..."
	@set -a && [ -f .env ] && . ./.env; set +a && go test -v -race ./...

test-blockfrost:
	@echo "Running blockfrost tests..."
	@set -a && [ -f .env ] && . ./.env; set +a && go test -v -race ./blockfrost

test-kupmios:
	@echo "Running kupmios tests..."
	@set -a && [ -f .env ] && . ./.env; set +a && go test -v -race ./kupmios

test-utxorpc:
	@echo "Running utxorpc tests..."
	@set -a && [ -f .env ] && . ./.env; set +a && go test -v -race ./utxorpc

test-match:
	@echo "Running test: $(TEST)..."
	@go test -run $(TEST) -v ./...

format: golines
	@go fmt ./...
	@gofmt -s -w $(GO_FILES)

golines:
	@golines -w --ignore-generated --chain-split-dots --max-len=80 --reformat-tags .

lint:
	@echo "Running golangci-lint..."
	@golangci-lint run

lint-fix:
	@echo "Running golangci-lint with auto-fix..."
	@golangci-lint run --fix

clean:
	@go clean -testcache
	@rm -f $(BINARIES)