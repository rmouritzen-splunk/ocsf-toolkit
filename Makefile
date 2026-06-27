base_dir := ${CURDIR}
build_dir := ${base_dir}/build
dist_dir := ${base_dir}/dist
coverage_out := coverage.out
coverage_percentages_out := coverage-percentages.out
go_vet_out := go-vet.out
target_platforms := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64
VERSION ?= dev

.PHONY: build
build: build-ocsf-toolkit

.PHONY: build-all-platforms
build-all-platforms: build-ocsf-toolkit-all-platforms

.PHONY: build-dir
build-dir: | $(build_dir)

${build_dir}:
	@echo "Creating build directory"
	mkdir $@

.PHONY: build-ocsf-toolkit
build-ocsf-toolkit: build-dir
	@echo "Building ocsf-toolkit"
	CGO_ENABLED=0 go build -C cmd/ocsf-toolkit -o ${build_dir} -trimpath

.PHONY: build-ocsf-toolkit-all-platforms
build-ocsf-toolkit-all-platforms: build-dir
	@echo "Building ocsf-toolkit for all target platforms"
	@BUILD_DIR="${build_dir}" TARGET_PLATFORMS="${target_platforms}" VERSION="${VERSION}" scripts/build-ocsf-toolkit-all-platforms.sh

.PHONY: lint
lint:
	@echo "Running golangci-lint"
	command -v golangci-lint >/dev/null 2>&1 || ( \
		echo "ERROR: golangci-lint is required for make lint."; \
		exit 1 \
	)
	golangci-lint run

.PHONY: gofmt-check
gofmt-check:
	@echo "Checking Go formatting"
	test -z "$$(gofmt -l .)"

.PHONY: govet
govet:
	@echo "Running go vet"
	go vet ./...

.PHONY: test
test:
	@echo "Running unit tests with coverage"
	go test -v -cover -covermode=count -coverprofile=${coverage_out} -coverpkg ./... ./...

.PHONY: coverage
coverage: test
	@echo "Generating coverage report"
	go tool cover -func ${coverage_out} > ${coverage_percentages_out}
	@echo
	@echo "Total Statement Coverage:"
	@tail -c 6 ${coverage_percentages_out}
	@echo

.PHONY: gotidy-check
gotidy-check:
	@echo "Checking go.mod and go.sum"
	go mod tidy -diff

.PHONY: gotidy
gotidy:
	@echo "Tidying Go module files"
	rm -rf go.sum
	go mod tidy

.PHONY: gofmt
gofmt:
	@echo "Formatting Go files"
	gofmt -w .

.PHONY: verify
verify: gotidy-check gofmt-check lint test govet build

.PHONY: verify-all-platforms
verify-all-platforms: gotidy-check gofmt-check lint coverage govet build-all-platforms

.PHONY: package-dist
package-dist: build-all-platforms
	@echo "Packaging release artifacts"
	@BUILD_DIR="${build_dir}" DIST_DIR="${dist_dir}" TARGET_PLATFORMS="${target_platforms}" VERSION="${VERSION}" scripts/package-dist.sh

.PHONY: package
package: gotidy-check gofmt-check lint coverage govet package-dist

.PHONY: clean
clean:
	@echo "Removing generated build and report files"
	rm -rf build
	rm -rf dist
	rm -f ${coverage_out} ${coverage_percentages_out} ${go_vet_out}
