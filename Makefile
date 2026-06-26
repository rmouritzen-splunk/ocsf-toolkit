base_dir := ${CURDIR}
build_dir := ${base_dir}/build
coverage_out := coverage.out
coverage_percentages_out := coverage-percentages.out
go_vet_out := go-vet.out

.PHONY: build
build: build-ocsf-toolkit

.PHONY: build-dir
build-dir: | $(build_dir)

${build_dir}:
	@echo "Creating build directory"
	mkdir $@

.PHONY: build-ocsf-toolkit
build-ocsf-toolkit: build-dir
	@echo "Building ocsf-toolkit"
	CGO_ENABLED=0 go build -C cmd/ocsf-toolkit -o ${build_dir} -trimpath

.PHONY: lint
lint:
	@echo "Checking Go formatting"
	test -z "$$(gofmt -l .)"
	@echo "Running golangci-lint"
	command -v golangci-lint >/dev/null 2>&1 || ( \
		echo "ERROR: golangci-lint is required for make lint."; \
		exit 1 \
	)
	golangci-lint run

.PHONY: govet
govet:
	@echo "Running go vet"
	go vet ./...

.PHONY: vet-ci
govet-ci:
	@echo "Generating go vet report"
	go vet ./... > ${go_vet_out} 2>&1 || (cat ${go_vet_out}; exit 1)
	cat ${go_vet_out}

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
verify: gotidy-check lint test govet build

.PHONY: verify-ci
verify-ci: gotidy-check lint coverage govet-ci build

.PHONY: clean
clean:
	@echo "Removing generated build and report files"
	rm -rf build
	rm -f ${coverage_out} ${coverage_percentages_out} ${go_vet_out}
