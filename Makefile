coverage_out := coverage.out
coverage_percentages_out := coverage-percentages.out
VERSION ?= dev
minimum_go_version := 1.25.13

export VERSION
export GOEXPERIMENT

.DEFAULT_GOAL := dev

# --- Build ---

.PHONY: build
build:
	@echo "Building ocsf-toolkit with GOEXPERIMENT=${GOEXPERIMENT}"
	@mkdir -p build
	CGO_ENABLED=0 go build -C cmd/ocsf-toolkit -o "${CURDIR}/build" -trimpath

.PHONY: build-all-platforms
build-all-platforms:
	@echo "Building ocsf-toolkit for all target platforms"
	@scripts/build-all-platforms.sh

# --- Module tidiness ---

.PHONY: gotidy-check
gotidy-check:
	@echo "Checking go.mod and go.sum"
	go mod tidy -diff
	go mod tidy -C tools -diff

.PHONY: gotidy
gotidy:
	@echo "Tidying Go module files"
	rm -rf go.sum
	go mod tidy
	rm -f tools/go.sum
	go mod tidy -C tools

# --- Checks ---

.PHONY: lint
lint:
	@echo "Running golangci-lint"
	@command -v golangci-lint >/dev/null 2>&1 || ( \
		echo "ERROR: golangci-lint is required for make lint. See README.md's Development section for install instructions."; \
		exit 1 \
	)
	golangci-lint run

.PHONY: lint-audit
lint-audit:
	@echo "Running extended golangci-lint audit"
	@command -v golangci-lint >/dev/null 2>&1 || ( \
		echo "ERROR: golangci-lint is required for make lint-audit. See README.md's Development section for install instructions."; \
		exit 1 \
	)
	golangci-lint run --enable exhaustive,gocognit,maintidx

.PHONY: vulncheck
vulncheck:
	@echo "Running govulncheck"
	@command -v govulncheck >/dev/null 2>&1 || ( \
		echo "ERROR: govulncheck is required for make vulncheck. See README.md's Development section for install instructions."; \
		exit 1 \
	)
	govulncheck ./...

.PHONY: govet
govet:
	@echo "Running go vet"
	go vet ./...

.PHONY: gofmt-check
gofmt-check:
	@echo "Checking Go formatting"
	test -z "$$(gofmt -l .)"

.PHONY: gofmt
gofmt:
	@echo "Formatting Go files"
	gofmt -w .

.PHONY: goimports-check
goimports-check:
	@echo "Checking Go import formatting via goimports"
	@command -v goimports >/dev/null 2>&1 || ( \
		echo "ERROR: goimports is required for make goimports-check. See README.md's Development section for install instructions."; \
		exit 1 \
	)
	@files="$$(goimports -l .)"; \
	if [ -n "$$files" ]; then \
		echo "$$files"; \
		echo "Import formatting differs above. Preview a fix with: goimports -d <file>"; \
		exit 1; \
	fi

.PHONY: goimports
goimports:
	@echo "Formatting Go imports"
	@command -v goimports >/dev/null 2>&1 || ( \
		echo "ERROR: goimports is required for make goimports. See README.md's Development section for install instructions."; \
		exit 1 \
	)
	goimports -w .

.PHONY: check
check: lint gotidy-check gofmt-check vulncheck govet

.PHONY: check-all
check-all: check goimports-check

# --- Tests ---

.PHONY: test
test:
	@echo "Running unit tests with the current Go toolchain and JSON v2"
	GOEXPERIMENT=jsonv2 go test ./...

.PHONY: test-coverage
test-coverage:
	@echo "Running unit tests with the current Go toolchain and default JSON implementation (with coverage)"
	GOEXPERIMENT= go test -v -cover -covermode=count -coverprofile=${coverage_out} -coverpkg ./... ./...
	@echo "Generating coverage report"
	go tool cover -func ${coverage_out} > ${coverage_percentages_out}
	@echo
	@echo "Total Statement Coverage:"
	@tail -c 6 ${coverage_percentages_out}
	@echo

.PHONY: test-compatibility
test-compatibility:
	@echo "Running unit tests with the current Go toolchain and default JSON implementation"
	GOEXPERIMENT= go test ./...
	@echo "Running unit tests with the current Go toolchain and JSON v2"
	GOEXPERIMENT=jsonv2 go test ./...
	@echo "Running unit tests with Go ${minimum_go_version} and the default JSON implementation"
	GOTOOLCHAIN=go${minimum_go_version} GOEXPERIMENT= go test ./...
	@echo "Running unit tests with Go ${minimum_go_version} and JSON v2"
	GOTOOLCHAIN=go${minimum_go_version} GOEXPERIMENT=jsonv2 go test ./...

.PHONY: test-race
test-race:
	@echo "Running unit tests with the race detector"
	go test -race ./...

.PHONY: test-latest-release-tag
test-latest-release-tag:
	@echo "Testing latest release tag selection"
	@scripts/test-latest-release-tag.sh

.PHONY: test-benchmark-compare
test-benchmark-compare:
	@echo "Testing benchmark comparison arguments"
	@scripts/test-benchmark-compare.sh

.PHONY: test-make-variable-safety
test-make-variable-safety:
	@echo "Testing Make variable handling"
	@scripts/test-make-variable-safety.sh

.PHONY: test-all
test-all: test-compatibility test-coverage test-race test-latest-release-tag test-benchmark-compare \
	test-make-variable-safety

# --- Orchestration ---

.PHONY: dev
dev: export GOEXPERIMENT := jsonv2
dev: check test build

.PHONY: all
all: check-all test-all build-all-platforms

# --- Packaging ---

.PHONY: package
package: export GOEXPERIMENT := jsonv2
package: all
	@echo "Packaging release artifacts"
	@scripts/package.sh

# --- Housekeeping ---

.PHONY: clean
clean:
	@echo "Removing generated build and report files"
	rm -rf build
	rm -rf dist
	rm -f ${coverage_out} ${coverage_percentages_out}
