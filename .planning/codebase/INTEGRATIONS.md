# External Integrations

**Analysis Date:** 2026-04-10

## APIs & External Services

**Not Applicable**
- GoBatch is a library, not an application with external service integrations
- Library users define their own external integrations via Source and Processor interfaces

## Data Storage

**Databases:**
- Not built-in - Library users implement custom Source interface for database connectivity

**File Storage:**
- Not built-in - Library users implement custom Source interface for file system access

**Caching:**
- Not built-in - Library users implement custom Processor interface for cache operations

## Authentication & Identity

**Auth Provider:**
- Not applicable - GoBatch is a library without built-in authentication

## Monitoring & Observability

**Error Tracking:**
- Errors returned through error channels
- Error wrapping provided: `SourceError`, `ProcessorError`, and `ItemError` (with `ItemID`) types in `batch/errors.go`
- Users can implement custom error handling via channels

**Logs:**
- Logging not built-in - Library users implement custom logging via Processor interface
- Error chain maintained via `errors.Unwrap()` for debugging

## CI/CD & Deployment

**Hosting:**
- GitHub repository: `github.com/MasterOfBinary/gobatch`
- Package registry: pkg.go.dev (automatic via Go module)

**CI Pipeline:**
- GitHub Actions (`.github/workflows/go.yml`)
- Triggers: Pushes to master/travis-test, pull requests to master
- Matrix testing: Go versions 1.25.x and 1.26.x

**Coverage Integration:**
- Codecov
  - Service: `codecov/codecov-action@v5`
  - Auth: `CODECOV_TOKEN` secret
  - Purpose: Track test coverage metrics

## Environment Configuration

**Required env vars:**
- `CODECOV_TOKEN` - For coverage uploads (CI only, secret)

**Secrets location:**
- GitHub Secrets (repository-level)
- Not stored in codebase

## Webhooks & Callbacks

**Incoming:**
- Not applicable - Library without HTTP endpoints

**Outgoing:**
- Codecov webhook (automatic coverage uploads via CI)

## Extensibility Points

**User-Implemented Integrations:**
- Custom `Source[T]` implementations: `source/doc.go`
  - Database sources
  - API sources
  - File system sources
  - Message queue sources

- Custom `Processor[T]` implementations: `processor/doc.go`
  - Database writes
  - API calls
  - File writes
  - Cache operations
  - External service notifications

---

*Integration audit: 2026-04-10*
