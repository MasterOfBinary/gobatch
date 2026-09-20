# Adversarial review of revision 1

Independent review. The full report is preserved here. Verdict: not
approvable as written for `batch` or `flow`. Ranked blockers that
revision 2 must close:

1. Formation unspecified once workers exist (`MaxItems`, `WithQueue`,
   greedy drain, and “one Do per worker” cannot all be true).
2. Same `Policy`, two grouping laws (Batcher greedy vs Loader light load).
3. `Do`/`Add` before `Run` deadlocks; the 6.5 example never starts
   `Loader.Run`.
4. Shared Loader `Run` ctx vs per-run `Do`/`flow.Run` ctx.
5. Three stop rules, no winner; the two-context sample self-aborts.
6. Re-entry into the same worker (`Do` from a LoadHandler, `Add` from a
   Handler, `flow.Run` from a node).
7. Shutdown budget leaks waiters by spec (`Do` not settled on expiry).
8. “Exactly one terminal outcome” is not a Call state machine.
9. Flow dispatch, `If`, `Result`, and bounds cannot be implemented from
   the text.
10. `Run` panics if called twice (v0.5 already had `ErrBatchUsed`).
11. Silent `MinItems` rewrite; `SetPolicy` punches a hole in `NewLoader`.
12. Default 30s handler timeout invents persist failure.
13. `flow` depends on nothing in `batch` is already false (`Event`,
    `ErrClosed`).
14. Close-drain grouping and “started” undefined.

Rules the review said to keep are listed in [REVIEWS.md](../REVIEWS.md).
