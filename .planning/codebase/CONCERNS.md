# Concerns

## High Priority

### Panic-based Error Handling
- **Location:** `batch/batch.go` — `WithBufferConfig()` and `Go()`
- **Issue:** Uses `panic()` for invalid configuration and double-start detection instead of returning errors
- **Risk:** Callers cannot gracefully handle these failures; panics propagate up and crash the program
- **Suggestion:** Return errors instead of panicking, or document panic behavior clearly

## Medium Priority

### ~~Unused Context Parameter in waitForItems~~ — RESOLVED (2026-04-29)
- **Location:** `batch/batch.go` — `waitForItems()` method
- **Resolution:** ctx is now wired into the select. On `ctx.Done()` `waitForItems` returns any partial batch immediately, then drains remaining buffered items one at a time so already-read items are not dropped. Subject to the cancellation contract documented in ARCHITECTURE.md (the source must close its channels in response to ctx — sources that ignore ctx will still block).

### Silent Configuration Adjustment
- **Location:** `batch/config.go` — `fixConfig()` function
- **Issue:** Silently adjusts invalid configuration values (e.g., negative times, zero items) without warning
- **Risk:** Users may not realize their configuration was modified; debugging unexpected behavior is harder
- **Suggestion:** Log warnings when configuration values are adjusted, or return validation errors

### MaxTime Timer Edge Cases — PARTIALLY ADDRESSED (2026-04-29)
- **Location:** `batch/batch.go` — timer handling in waitForItems
- **Resolved part:** Timer leaks are fixed. `time.After` was replaced with `time.NewTimer` plus `defer Stop()` so timers cannot leak when a batch returns before its timer fires.
- **Remaining issue:** `waitForItems` still resets `maxTimer` indefinitely when it fires with an empty batch (`maxTimer.Reset(config.MaxTime)`). Under a pathological source that produces no items, the loop will spin every `MaxTime` interval. The cancellation path closes one escape hatch, but an explicit idle-timeout policy is still worth considering.

### Lock Contention in ExecuteBatches
- **Location:** `batch/helpers.go` — `ExecuteBatches()` function
- **Issue:** Uses shared mutex to collect errors from multiple concurrent batches
- **Risk:** Under high concurrency with many errors, lock contention could impact performance
- **Suggestion:** Use channel-based error collection or sync.Map for concurrent append

## Low Priority

### Processor Contract Documentation — PARTIALLY ADDRESSED (2026-04-29)
- **Issue:** The exact contract for Processor implementations (when to check item.Error, when to skip vs. process errored items) is implicit
- **Resolved part:** Per-item errors are now reported as `*ItemError` (with the failing `ItemID`) rather than a generic `*ProcessorError`. This makes the conceptual distinction between processor-wide and item-specific failures explicit at the type level.
- **Remaining issue:** The Processor godoc still does not formally specify whether processors should skip items with `Error != nil` or process them. Custom processors will continue to handle this inconsistently until the interface contract is documented.

### ~~ID Overflow Risk~~ — IMPLEMENTATION SIMPLIFIED (2026-04-29)
- **Location:** `batch/batch.go` — `doReader()` (was `doIDGenerator()`)
- **Status:** The dedicated `doIDGenerator` goroutine and channel are gone. IDs are now produced inline in `doReader` via `atomic.AddUint64(&b.nextID, 1) - 1`. The uint64 overflow property is unchanged — IDs still wrap to 0 after 2^64 items, which remains an acceptable risk in practice.

## Version Stability

- Project is explicitly v0 (pre-1.0) — breaking changes expected on master branch
- No tagged releases or semantic versioning in use
- API recently migrated to generics (commit `7a85eca`)

---

*Concerns analysis: 2026-04-10. Updated 2026-04-29 to reflect the cancellation-and-IDs refactor; the High-priority panic concern remains open.*
