# binmgr justfile - common development tasks

# Default recipe to display help
default:
    @just --list

# Build the binmgr binary
build:
    go build -o binmgr ./cmd/binmgr/...

# Build with verbose output
build-verbose:
    go build -v -o binmgr ./cmd/binmgr/...

# Build for multiple platforms
build-all:
    GOOS=linux GOARCH=amd64 go build -o binmgr-linux-amd64 ./cmd/binmgr/...
    GOOS=linux GOARCH=arm64 go build -o binmgr-linux-arm64 ./cmd/binmgr/...
    GOOS=darwin GOARCH=amd64 go build -o binmgr-darwin-amd64 ./cmd/binmgr/...
    GOOS=darwin GOARCH=arm64 go build -o binmgr-darwin-arm64 ./cmd/binmgr/...
    GOOS=windows GOARCH=amd64 go build -o binmgr-windows-amd64.exe ./cmd/binmgr/...

# Run all tests
test:
    go test ./...

# Run tests with verbose output
test-verbose:
    go test -v ./...

# Run tests with race detection
test-race:
    go test -race ./...

# Run only short tests
test-short:
    go test -short ./...

# Run tests for a specific package (e.g., just test-pkg pkg/backend)
test-pkg package:
    go test -v ./{{package}}

# Run a specific test (e.g., just test-one pkg/backend TestNewBinmgrManifest)
test-one package testname:
    go test -v ./{{package}} -run {{testname}}

# Generate test coverage report
coverage:
    go test -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out

# Generate and display HTML coverage report
coverage-html:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report generated: coverage.html"

# Show coverage percentage summary
coverage-summary:
    go test -coverprofile=coverage.out ./... 2>&1 | grep -E "coverage:|PASS|FAIL"
    @echo ""
    @go tool cover -func=coverage.out | tail -1

# Run tests with coverage and open HTML report in browser (Linux)
coverage-browse:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    xdg-open coverage.html 2>/dev/null || open coverage.html 2>/dev/null || echo "Please open coverage.html manually"

# Format all Go code
fmt:
    go fmt ./...

# Run go vet for static analysis
vet:
    go vet ./...

# Run golangci-lint (if installed)
lint:
    golangci-lint run ./...

# Run all checks (fmt, vet, test)
check: fmt vet test

# Clean build artifacts and test files
clean:
    rm -f binmgr binmgr-* coverage.out coverage.html
    go clean -testcache

# Install binmgr to $GOPATH/bin
install:
    go install ./cmd/binmgr/...

# Run tests and generate coverage in one command
test-cover: test coverage-summary

# Benchmark tests
bench:
    go test -bench=. -benchmem ./...

# Download and tidy dependencies
deps:
    go mod download
    go mod tidy

# Verify dependencies
deps-verify:
    go mod verify

# Show outdated dependencies
deps-outdated:
    go list -u -m all

# Run all quality checks before committing
pre-commit: fmt vet test-cover

# Quick check (format and fast tests)
quick: fmt test-short
