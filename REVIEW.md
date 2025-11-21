# GoBatch Code Review & Roadmap

## Executive Summary
GoBatch is a promising library for batch processing in Go. It has a clean structure and clear separation of concerns (Source, Batch, Processor). However, the current implementation (v0.3.0) contains a few critical bugs related to concurrency and timing, and it relies heavily on `interface{}`, missing out on the type safety benefits of Go 1.18+ Generics.

This document outlines the identified bugs, suggested improvements, and a strategic roadmap to evolve the project into a robust, "sync-like" batching solution.

---

## 1. Critical Bugs & Issues

### 1.1. Busy Loop in `doReader`
**Severity:** Critical
**Location:** `batch/batch.go` (Method `doReader`)

When reading from the Source, `doReader` uses a `select` statement on the `out` and `errs` channels. If one channel is closed by the Source while the other remains open, the `select` statement will repeatedly choose the closed channel (which returns immediately), resulting in a busy loop that consumes 100% CPU until the other channel is closed.

**Fix:** Set the channel variable to `nil` after detecting it is closed. `select` ignores nil channels.

```go
// Current
case data, ok := <-out:
    if !ok {
        outClosed = true
        continue
    }

// Recommended Fix
case data, ok := <-out:
    if !ok {
        outClosed = true
        out = nil // Disable this case
        continue
    }
```

### 1.2. `MaxTime` Logic Flaw
**Severity:** High
**Location:** `batch/batch.go` (Method `waitForItems`)

The `MaxTime` timer starts immediately when `waitForItems` is called. If the system is idle for longer than `MaxTime`, the timer fires on an empty batch and is discarded. When the first item finally arrives, the timer is already "used," and the item sits in the buffer until `MinItems` is reached. This violates the latency guarantee for the first batch after an idle period.

**reproduction:**
1. Set `MaxTime = 100ms`, `MinItems = 10`.
2. Wait 200ms (Timer expires).
3. Send 1 item.
4. Wait 200ms.
5. Result: Item is NOT processed.

**Fix:** The `MaxTime` timer should be started (or reset) only when the first item of a batch arrives.

### 1.3. `Transform` Configuration Default Mismatch
**Severity:** Low
**Location:** `processor/transform.go`

The documentation for `Transform` states that `ContinueOnError` defaults to `true`. However, since it is a boolean field, the zero value is `false`. Users initializing the struct will get "stop on error" behavior by default, contradicting the docs.

**Fix:**
- Change the field to `StopOnError` (default false = continue).
- Or use a constructor `NewTransform(...)` that sets the default.
- Or use `*bool` to detect absence.

---

## 2. Code Improvements

### 2.1. Context in `TransformFunc`
The `TransformFunc` signature is `func(data interface{}) (interface{}, error)`. It lacks `context.Context`, making it impossible to perform cancellable I/O operations (e.g., database lookups) within a transform processor.

**Suggestion:** Update signature to `func(ctx context.Context, data interface{}) (interface{}, error)`.

### 2.2. Missing Context Check in `waitForItems`
`waitForItems` loops indefinitely waiting for items or timers. While it relies on the `items` channel closing to exit, adding an explicit `case <-ctx.Done():` would make the shutdown more robust and responsive, especially if the internal components are stalled.

### 2.3. Standardize Error Handling
Currently, `batch.go` manually wraps errors. Consider using a standardized error wrapping helper or Go 1.20 `errors.Join` (if upgrading Go version) to manage multiple errors more cleaner.

---

## 3. Roadmap

### 3.1. Phase 1: Stability & Generics (Immediate)
**Goal:** Fix bugs and modernize the codebase.

1.  **Fix identified bugs:** `doReader` busy loop, `MaxTime` logic, `Transform` defaults.
2.  **Migrate to Generics (Go 1.18+):**
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
