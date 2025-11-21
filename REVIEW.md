# GoBatch Code Review & Roadmap

## Executive Summary
GoBatch is a promising library for batch processing in Go. It has a clean structure and clear separation of concerns (Source, Batch, Processor).

**Status Update:** As of the latest update, the critical bugs identified during the review (Busy Loop, MaxTime Logic, Configuration Defaults) have been **fixed**. The project is now stable and ready for the next phase of development.

This document outlines the resolved issues and a strategic roadmap to evolve the project into a robust, "sync-like" batching solution.

---

## 1. Resolved Issues (Fixed)

### 1.1. Busy Loop in `doReader`
**Issue:** When reading from the Source, `doReader` used a `select` statement that spun in a tight loop if one channel was closed while the other remained open.
**Resolution:** The code now correctly sets closed channels to `nil`, disabling them in the `select` statement and preventing the busy loop.

### 1.2. `MaxTime` Logic Flaw
**Issue:** The `MaxTime` timer would expire on an empty batch and fail to restart, causing the first item of a subsequent batch to wait indefinitely (or until `MinItems` was met).
**Resolution:** The timer logic in `waitForItems` now restarts the `MaxTime` timer if it expires while the batch is empty, ensuring the latency guarantee is maintained.

### 1.3. `Transform` Configuration Default Mismatch
**Issue:** The `ContinueOnError` field defaulted to `false` (Stop), contradicting the documentation which claimed it defaulted to `true` (Continue).
**Resolution:** The field has been renamed to `StopOnError` (default `false` = Continue), aligning the zero-value behavior with the intended default of continuing processing.

---

## 2. Code Improvements (Suggestions)

### 2.1. Context in `TransformFunc`
The `TransformFunc` signature is `func(data interface{}) (interface{}, error)`. It lacks `context.Context`, making it impossible to perform cancellable I/O operations (e.g., database lookups) within a transform processor.

**Suggestion:** Update signature to `func(ctx context.Context, data interface{}) (interface{}, error)`.

### 2.2. Standardize Error Handling
Currently, `batch.go` manually wraps errors. Consider using a standardized error wrapping helper or Go 1.20 `errors.Join` (if upgrading Go version) to manage multiple errors more cleaner.

---

## 3. Roadmap

### 3.1. Phase 1: Stability & Generics (Next Steps)
**Goal:** Modernize the codebase.

1.  **Migrate to Generics (Go 1.18+):**
    - Replace `interface{}` with `Batch[T any]`.
    - `Source[T]`, `Processor[T]`.
    - This will provide compile-time type safety and eliminate runtime type assertions (`data.(MyType)`).

### 3.2. Phase 2: "Sync-like" Batching (Request-Reply Pattern)
**Goal:** Support the primary use case of "Batching sync operations transparently" (e.g., Redis Pipeline).

To achieve "Redis Get(key) -> returns value" backed by a batcher, we need a mechanism to return results to the caller.

**Proposed Design:**
- Introduce a **Promise/Future** pattern.
- **Batched Request Wrapper:**
  ```go
  type Request[T, R any] struct {
      Input  T
      Result chan Result[R] // or a Promise struct
  }
  ```
- **Usage Example:**
  ```go
  // User Code
  func GetUser(id int) (*User, error) {
      req := batch.NewRequest[int, *User](id)
      batcher.Submit(req)
      return req.Await(ctx) // Blocks until result is ready
  }
  ```
- **Processor Implementation:**
  The processor receives a batch of `Request` objects, performs the bulk operation (e.g., `SELECT * FROM users WHERE id IN (...)`), and then maps the results back to the individual `Request` objects by closing their result channels.

### 3.3. Phase 3: Ecosystem & Observability
**Goal:** Make the library production-ready.

1.  **Standard Processors:**
    - `Deduplicate[T]`: Drop duplicate items in a batch.
    - `Aggregate[T]`: Combine items (reduce).
    - `Retry`: Decorator for processors to retry on specific errors.
2.  **Standard Sources:**
    - `SliceSource`: Process a static slice.
    - `PollSource`: Periodically poll an external API.
3.  **Observability:**
    - Expose metrics: `BatchSize`, `ProcessingTime`, `ErrorRate`.
    - OpenTelemetry integration hooks.
