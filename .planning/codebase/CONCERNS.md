# Concerns

## High Priority

### Panic-based Error Handling
- **Location:** `batch/batch.go` — `WithBufferConfig()` and `Go()`
- **Issue:** Uses `panic()` for invalid configuration and double-start detection instead of returning errors
- **Risk:** Callers cannot gracefully handle these failures; panics propagate up and crash the program
- **Suggestion:** Return errors instead of panicking, or document panic behavior clearly

## Medium Priority

### Unused Context Parameter in waitForItems
- **Location:** `batch/batch.go` — `waitForItems()` method
- **Issue:** Context parameter is accepted but not used for cancellation within the batching logic
- **Risk:** Batching cannot be interrupted mid-wait; relies on channel closure instead
- **Suggestion:** Wire context cancellation into the select statements in waitForItems

### Silent Configuration Adjustment
- **Location:** `batch/config.go` — `fixConfig()` function
- **Issue:** Silently adjusts invalid configuration values (e.g., negative times, zero items) without warning
- **Risk:** Users may not realize their configuration was modified; debugging unexpected behavior is harder
- **Suggestion:** Log warnings when configuration values are adjusted, or return validation errors

### MaxTime Timer Edge Cases
- **Location:** `batch/batch.go` — timer handling in waitForItems
- **Issue:** Repeated MaxTime timeouts with no items could create timer leak or spinning behavior
- **Risk:** Under pathological conditions (source that never sends items), timer resets indefinitely
- **Suggestion:** Consider adding an idle timeout or backoff mechanism

### Lock Contention in ExecuteBatches
- **Location:** `batch/helpers.go` — `ExecuteBatches()` function
- **Issue:** Uses shared mutex to collect errors from multiple concurrent batches
- **Risk:** Under high concurrency with many errors, lock contention could impact performance
- **Suggestion:** Use channel-based error collection or sync.Map for concurrent append

## Low Priority

### Processor Contract Documentation
- **Issue:** The exact contract for Processor implementations (when to check item.Error, when to skip vs. process errored items) is implicit
- **Risk:** Custom processor implementations may handle errors inconsistently
- **Suggestion:** Document the expected contract explicitly in the Processor interface godoc

### ID Overflow Risk
- **Location:** `batch/batch.go` — `doIDGenerator()`
- **Issue:** Uses uint64 counter for item IDs; overflows after 2^64 items
- **Risk:** Extremely unlikely in practice but IDs would wrap to 0
- **Suggestion:** Acceptable risk; document the limitation if needed

## Version Stability

- Project is explicitly v0 (pre-1.0) — breaking changes expected on master branch
- No tagged releases or semantic versioning in use
- API recently migrated to generics (commit `7a85eca`)

---

*Concerns analysis: 2026-04-10*
