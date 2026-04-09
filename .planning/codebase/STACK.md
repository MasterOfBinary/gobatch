# Technology Stack

**Analysis Date:** 2026-04-10

## Languages

**Primary:**
- Go 1.18+ - Core library implementation

**Supported:**
- Go 1.25, 1.26 - Tested via CI/CD pipeline (GitHub Actions)

## Runtime

**Environment:**
- Go compiler and runtime (no external runtime dependencies)

**Package Manager:**
- Go modules (go mod)
- Lockfile: Not applicable (no external dependencies)

## Frameworks

**Core:**
- None - Pure Go standard library implementation

**Testing:**
- Go's built-in `testing` package - Used for all unit tests
- Custom test helpers in `batch/testhelpers_test.go` - Test utilities

**Build/Dev:**
- `gofmt` - Code formatting (enforced in CI)
- `golangci-lint` v1.64.8 - Linting and code quality
- `go vet` - Basic static analysis

## Key Dependencies

**Zero External Dependencies:**
- GoBatch has no external production dependencies
- All functionality built on Go standard library only
- Standard library packages used:
  - `context` - Context management
  - `errors` - Error handling and wrapping
  - `sync` - Synchronization primitives (Mutex)
  - `time` - Timing and duration handling
  - `fmt` - String formatting

## Configuration

**Environment:**
- No environment configuration required for the library itself
- CI/CD environment: Ubuntu latest (via GitHub Actions)

**Build:**
- Standard Go build commands
- No build configuration files (go.mod/go.sum only)

## Platform Requirements

**Development:**
- Go 1.18 or later
- gofmt (included with Go)
- golangci-lint (for linting, installed via CI workflow)
- Unix-like environment for bash scripts

**Production:**
- Go 1.18 or later
- No external runtime dependencies
- Can be compiled for any platform supported by Go

## Testing Infrastructure

**Test Execution:**
- Run all tests with race detection and coverage: `go test -race -coverprofile=coverage.txt -covermode=atomic ./...`
- Coverage tracking via Codecov (secrets-based authentication)
- Tests located alongside source files: `*_test.go` pattern

**Linting:**
- Format check: `gofmt -l $(git ls-files '*.go')`
- Lint: `golangci-lint run --timeout=3m`

## CI/CD Platform

**GitHub Actions:**
- Workflow file: `.github/workflows/go.yml`
- Triggers: Push to master/travis-test branches, pull requests to master
- Jobs:
  - Code formatting verification
  - Tests with race detection and coverage
  - Linter checks
  - Coverage upload to Codecov

---

*Stack analysis: 2026-04-10*
