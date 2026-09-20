# Adversarial review, pass 2: GoBatch redesign proposal revision 2

Re-read in full. Revision 2 removed or repaired every blocker from pass 1 (implicit ordinals, `Each`, the three-way retry contradiction, the blocking scheduler, the miswired drain example, `Close`/`Incomplete`, `Pool`, `Deadline`, the `Pool` redeclaration). What follows is what the new text introduces or leaves open, ranked. Severity: blocker / major / minor.

---

## 1. BLOCKER — Retry as written is dead code: the task is failed (handle resolves, dependents get `DependencyError`) before it is retried (§5.7, §5.9)

§5.7: "a handler error **fails every unfinished task** in the group with that error. If the kind has a policy and `Retry(err)` is true, the unfinished remainder is re-invoked". §5.9's table agrees: `in handler → failed`, then `failed (retryable) → backing off`. §5.3: "A dependency failing fails its dependents ... without running them." §5.2: `Complete` is "first answer wins; later answers are ignored".

**Scenario.** Status group, RPC times out, handler returns a transient error. Under the text, all 256 tasks enter `failed`; their handles resolve with `*TaskError`; every `After` dependent is failed with `*DependencyError`; then the group is retried, succeeds, and each `Complete` is the *second* answer, ignored and counted. Retry never produces a result anybody can see.

**Fix.** Split the state: `in handler → retrying` (handle not resolved, dependents untouched) when `Retry(err)` holds and attempts/budget remain; `in handler → failed` otherwise. Say explicitly that a handle resolves and dependents are notified only on a terminal failure. Same for `Fail(err)` retried alone.

## 2. BLOCKER — `MaxOpen` bounds nothing in either consumer mapping: `After` outside a handler creates rootless tasks, and `After` inside a handler cannot name its parent (§5.3, §5.5, §6.1, §6.2)

"Each task carries its root; a per-root counter of unfinished descendants is what `MaxOpen` reads." Which root does `k.After(key, deps, build)` carry?

- Called from the reader goroutine with deps `[]Dep{root.At(s)}` (§6.1, all three kinds) or `[]Dep{blocks.Handle(slotKey)}` (§6.2): the deps are a watermark and a promise; neither has a root. Every task in both examples is rootless, so `MaxOpen` counts zero and "memory is bounded by `MaxOpen` times the largest subtree" is false for the proposal's own code.
- Called from inside a handler serving a group of N tasks: `After` has no `*Task` receiver and no ctx-carried task identity, so the library cannot know which of the N tasks (with N different roots) spawned the child. The "handler calling `After` per element" fan-out that replaced `Each` is exactly the case, and it is unattributable.
- Fan-in across roots (`deps` from roots A and B): counted under A, B, or both? Unspecified. Under one, the other root "finishes" while its work still runs.
- Deps on a `Remember`ed finished handle: root already at zero; re-incrementing reopens a finished root.

**Fix.** Make root attribution explicit: `After` outside a handler takes `ctx` and is a root (subject to `MaxOpen`, may block; that is fine outside a handler); inside a handler, spawning goes through the task, `t.After(...)`, which inherits `t`'s root and never blocks. Count a fan-in task under every distinct root of its task deps (a task decrements each on finish). State that promise- and watermark-only deps make the task its own root.

## 3. MAJOR — "Re-invoked as one group with the same `batch.Group.Seq`" is not expressible on the public `batch.Batcher`, so "one `batch.Batcher` per kind" is not true as stated (§5.7, §4.2)

`Batcher.Add` is per item; `Seq` is assigned by the batcher at release; a retried group must (a) be handed to a worker as one unit, (b) not merge with newly ready tasks of the same kind, (c) carry the old `Seq` and `Attempt+1`, and (d) hold no worker during backoff. None of that is reachable through `Add`/`Policy`. Either `batch` grows an unexported-but-real group-injection path (and the doc says the workflow uses `batch` internals, with `internal/` layout), or `Batcher` exports something like `AddGroup(ctx, items []T, g Group)` that bypasses the policy. Pick one; the delivery plan's "`workflow` depends on `batch`" is otherwise a claim about a package boundary that cannot hold.

Related: the retried remainder keeps the original `Seq` but is a smaller slice. The consumer's ruling 11 needs "the journaled request for attempt 2 is the journaled request for attempt 1". §6.1 correctly narrows that to "when nothing in the group was answered", but the handler cannot tell whether it is looking at the full original group or a remainder. Add `Group.Original int` (or `Partial bool`) so the handler can refuse to journal a remainder under the old key.

## 4. MAJOR — The `Sequence` release rule has a permanent head-of-line hole: tasks that fail before release are never "released" (§5.6, §5.9)

Rule (b): a task with ordinal `n` releases once "every registered task with a smaller ordinal has been **released**". A task with ordinal `m < n` that is failed *before* release, by `*DependencyError` (a dep failed), by a `build` error (no: that is after release), or by the `Run`-return sweep, is never released. Everything behind it waits forever, and `Run` reports it all as `*IncompleteError` at close. Note that §5.6's own hedge ("depend only on things that always complete") does not cover dependency *failure*, which a computation can do.

Also unspecified in the same rule:
- Registration with an ordinal at or below the settled mark violates the consumer's own declaration and silently breaks ordering if `n+1` was already released. Must panic.
- Two tasks with the same ordinal (dedup returns the first handle for the same key, but different keys can share an ordinal): tie order undefined.
- A sequenced task retried alone: is its second release subject to the rule? It was already released, so it jumps the queue. Say "retries bypass the sequence" or forbid `Retry` with `Sequence`.
- "Released" is not a state in the §5.9 table. Define it as "moved from the sequence buffer to the kind's ready queue", scheduler-side, so it is independent of batcher pull timing.
- `Settle` monotonic? `Settle(5)` then `Settle(3)`.

**Fix.** Rule (b) becomes "every registered task with a smaller ordinal is released or terminally failed", `Ordinal(n)` with `n <= settled` panics, ties by registration order, retries bypass.

## 5. MAJOR — Readiness order is missing events and never says what "happened" means (§5.1, §5.6, §5.9)

"The order in which the event that completed their last dependency happened." Events that ready a task in this design: `Complete`, `Resolve`, `Advance`, **registration when every dep is already complete** (§5.3 "ready at once"), **backoff expiry** (§5.9 `backing off → ready`), and, for `Reject`/failure, nothing. The first three are listed; the fourth and fifth are not, and the §5.9 table has no row for `After`-with-satisfied-deps. "Happened" across goroutines (`Resolve` on the WS goroutine, `Complete` on a worker) is only meaningful as scheduler-lock acquisition order; say that, and say the ready position is assigned under that lock, else two implementers will disagree.

Two consequences worth a sentence each: `Advance(10)` readying `At(3)` and `At(7)` releases them in *registration* order, not watermark order, so a consumer that wants slot order must register in slot order; and determinism with `Workers: 1` is determinism of *release order*, not of group boundaries, which `MaxWait`/`MinWait` make time-dependent, so a handler whose output depends on group composition (a group commit) is not replay-stable.

## 6. MAJOR — `Promise` and `Remember` have no eviction; a long run retains every value (§5.3, §5.4, §6.2)

"A block arriving before any swap is `Resolve` on a key nobody has asked for yet" is stored until `Run` returns, and so is every resolved value with dependents, because "handles own results" and the promise keeps a handle per key. In §6.2 that is every confirmed block of a `live` run. `Remember` likewise keeps every finished key; the text does not say whether it keeps the result too (it must, if a later registration gets the handle and can call `Result`). Neither has `Forget`, a TTL, or a stated bound.

**Fix.** `Promise.Forget(key)` and `Kind.Forget(key)` (or `Remember` as a count with LRU), and a sentence stating the bound. Also: `Resolve` after `Run` returned returns what? §5.9 says "no-ops that return zero handles already failed"; `Resolve` returns `bool`.

## 7. MAJOR — Batchers pull, but `Policy` timers are defined on "queued"; the two are not reconciled (§4.1, §5.5)

`MaxWait`/`MinWait` are "since the group's first item was queued". A workflow batcher pulls from the scheduler's ready queue; if the timer starts at pull, a `statuses` task that sat ready for 150 ms then waits another 200 ms `MinWait`; if it starts at readiness, the batcher must receive readiness instants with each task, and the pull must be eager (into the pending group) rather than on worker-free. Say which. Also: a group's slice order at the handler is not promised to be release order anywhere in §4.2 ("handed to workers in release order" is about groups), and a `Sequence` is meaningless at the handler without it; and a group whose `build`s all fail is invoked with an empty slice or skipped, unspecified.

## 8. MAJOR — `Fatal` interplay is unspecified (§5.7, §4.2)

Does `Retry(err)` see a `Fatal`-wrapped error (it must not retry it)? Are the group's unfinished tasks failed with `err` or with `ctx.Err()` from the abort? `Fatal(nil)`? And `batch.GroupError.Attempt` says "1 unless a workflow retried the group", implying the batcher surfaces workflow handler errors, but the workflow's internal `batch.Handler` must always return nil or the batcher stops on the first task-level error. Say that `Fatal` bypasses the classifier, that the group's tasks fail with the unwrapped `err`, and drop the `GroupError.Attempt` remark or keep it honest.

## 9. MAJOR — `Close` does not quiesce a consumer that keeps calling `After` and `Resolve` (§5.2, §5.9)

`Close` "stops root submissions". §6.1's per-notification loop calls `Submit` once and `After` twice; after `Close`, `Submit` returns `ErrClosed` but `After` and `Resolve` keep registering and readying tasks, so "nothing is ready, in flight or backing off" may never hold and `Run` returns only on ctx. Also the sweep ("fails every task still pending ... before returning") races an `After` issued between the quiescence decision and the closed-flag flip; implementable only if the sweep and the flip are one critical section. State both: `After`/`Resolve`/`Advance` after `Close` are no-ops returning failed handles (not only after `Run` returns), and the sweep is atomic with the flip.

## 10. MINOR — `Watermark` is not bound to a workflow but `At(v)` is a scheduler dependency (§5.4)

`NewPromise(w, name)` takes the workflow; `NewWatermark()` does not. For `Advance` to ready tasks, either the watermark holds scheduler callbacks (watermark lock → scheduler lock) while `After` registers `At(v)` waiters (scheduler lock → watermark lock): the AB/BA pair is the first `-race`/deadlock bug the implementation will hit. Fix: register waiters outside the scheduler lock, or copy the waiter list before calling out, and write the lock order in the doc. The asymmetry with `Promise` should be explained or removed.

## 11. MINOR — `MaxOpen` plus a never-resolved promise stalls ingestion; the dedup check must precede the `MaxOpen` wait (§5.5)

Once root attribution is fixed (finding 2), a root with one descendant waiting on a promise never resolved stays open for the run; `MaxOpen` such roots and `Submit` blocks until ctx. The only escape is the consumer running its own timeout and calling `Reject`. Say so. Separately: a duplicate `Submit` must return the existing handle *without* waiting on `MaxOpen`, or a reader goroutine stalls on a no-op.

## 12. MINOR — §5.8 "a goroutine answering after return races the auto-fail and loses" is false as a race

It may win. Make it true: the worker marks the task closed under its lock before the auto-fail, so a late answer is deterministically ignored and counted. One sentence.

## 13. MINOR — Two attempt counters and a budget that can be exceeded by one attempt (§5.7)

`Task.Attempt` (task retried alone) and `batch.Group.Attempt` (group retried) diverge: a task retried alone lands in a fresh group with `Group.Attempt == 1` and `Task.Attempt == 2`. Say which the observer reports. `Budget` "measured from the task's readiness instant": first readiness, presumably, and checked at re-invocation, so the last attempt may run past it. Say both.

## 14. MINOR — Small contradictions and gaps

- §4.4 `Loader` zero policy coalesces only with `Workers: 1`; with `Workers: 4` every call under light load is its own group. Say the default is deliberate.
- §4.2 `Add` before `Run` blocks once the queue is full; `Close` before `Run` then no `Run` ever: the blocked `Add` returns only on ctx. Fine, but say it.
- §5.11 `PendingTask.Waiting Dep` is singular for a task with several unresolved deps, and `Dep` needs a `String` for `IncompleteError` to print.
- §5.10 the observer is "called synchronously on the goroutine that made the transition": inside the scheduler lock, an observer that calls `Resolve` (allowed by §5.8) self-deadlocks on a non-reentrant lock; outside it, observed order can differ from transition order. Say which.
- §5.3 `Result` inside `build` "returns at once" only for the task's own deps; on a captured foreign handle it blocks a worker. Say "only on deps".

---

## Verdict

Approvable for the `batch` package now; not yet for `workflow`. Findings 1 and 2 are spec bugs an implementer would faithfully reproduce: retry that can never surface a result, and a `MaxOpen` that counts nothing in the proposal's own examples. Fix them, then 3 (group re-injection must exist in `batch`'s API or the package boundary is fiction) and 4 (the sequence rule must count terminal failure as release). Findings 5, 7 and 9 are one paragraph each and belong in the §5.1/§5.9 text before code, because they are where two implementations would diverge.
