# Adversarial review: GoBatch redesign proposal (docs/redesign/PROPOSAL.md)

Reviewed against the v0.5 code (`batch/batch.go`, `batch/config.go`, `batch/helpers.go`) and a skim of the consumer's `internal/helius/scheduler.go`, `internal/solana/normalizer.go` and `internal/capture/journal.go` to check the proposal's claims about them. Findings are ranked; severity is blocker / major / minor.

---

## 1. BLOCKER — `InOrder` cannot order tasks it has not seen yet; the "byte-identical by construction" claim is false (§5.6, §6.2)

**Scenario.** `admit` is `InOrder()`. Roots 6 and 7 are `decode` tasks running on four workers. Decode 7 finishes first; its `Each` registers admit tasks with ordinals (7,0..k) and they become ready. Decode 6 is still running; its admit tasks (6,0..n) do not exist in any table yet because `Each` can only register them after the slice result is known. The reorder buffer sees only (7,·). Either it releases (7,0) now, in which case a replay where decode 6 happens to finish first produces a different admission order (the exact perturbation §5.6 says ordinals prevent), or it must know that "something with major 6 may still arrive", which the proposal gives it no way to know. "A ready task behind an unready one waits" is only defined over *registered* tasks; the hole is unregistered ones.

The consumer's determinism today comes from the normalizer running "one capture, in record order, on one goroutine" (normalizer.go:71). The proposal replaces that with four decode workers plus a reorder buffer that has no low-watermark, and then asserts the result is a pure function of record order. It is not.

**Smallest fix.** The scheduler must track, per root major, an outstanding count over the root's whole transitive subtree (increment on every registration reachable from it, decrement on every terminal state). An `InOrder` kind may release ordinal (m,·) only when every root with major < m has zero outstanding. That is a real low-watermark and it is implementable, but be honest about the cost: `admit` for record 7 waits until *every* task of record 6's subtree, in every kind, is terminal, including a `confirmed` fetch with a two-minute retry budget if it is in the subtree. **Alternative I would prefer:** drop implicit ordinals entirely and make the buffer gate-driven: `InOrderBy(g *Gate)` releases tasks with ordinal ≤ `g.Value()`, and the consumer advances `g` when it knows record n's continuations are all registered (it knows: it is the one calling `Each`). That keeps "the consumer says when the world advanced" and reuses `Gate`.

## 2. BLOCKER — `Each` as specified cannot exist under the "continuations never block" rule (§5.5, §6.2)

`func Each[A, In, Out](k, dep Handle[[]A], key func(A) Key, build func(A) (In, error)) []Handle[Out]` returns a slice whose length is `len(dep's result)`, which is unknown until `dep` completes. So `Each` either blocks until the dependency completes (§6.2 calls it from the reader loop right after `Submit(decode)`, which would serialize the reader on decode and make `Workers(runtime.NumCPU())` pointless; called from a handler it is the handler-waits-on-another-task deadlock with `Workers(1)` or a shared `Pool(2)`), or it returns an empty/partial slice, or it returns before registering anything and the ordinals in finding 1 become even less knowable. On a failed dependency it has no defined return at all.

**Fix.** `Each` must return one handle: `Handle[[]Out]` (register-on-completion fan-out plus gather), or be replaced by a handler-side `Spawn` on a `*Task` that registers children before the parent completes. Either form is non-blocking and gives `Each`'s children a registration point that the low-watermark in finding 1 can count.

## 3. BLOCKER — Retry semantics contradict the task-completion contract (§5.2, §5.7, Q7)

Three rules apply to the same event and disagree:
- §5.2: "unfinished tasks are failed when the handler returns".
- §5.7 task granularity: a task failed with a retryable error is re-queued individually and "may land in a different group".
- §5.7 group granularity: a handler that returns an error has the *whole* group re-invoked, "same tasks, same order, same Group ID".

**Scenario.** Status handler completes calls 0..99 with replies, then the RPC for the remainder fails and it returns `err`. Under rule 1, calls 100..255 are failed with `err`; under rule 2, if `err` is transient they are re-queued individually; under rule 3 the entire group including 0..99 runs again, `Complete` is called twice on 0..99 (panic? ignored? second write to a resolved handle?) and their dependents, already released, are not un-released. Q7 phrases this as a nicety; it is an unspecified state machine at the core of the scheduler.

**Fix.** Pick one: (a) group retry is legal only if the handler answered nothing, otherwise the handler error fails the unanswered tasks and task-level retry applies; or (b) forbid `Complete`/`Fail` before an error return (panic in `-race` test builds). Define `Complete` twice and `Complete` after return as a documented panic or first-wins `sync.Once`, and say which.

## 4. MAJOR — "Dedup by first receipt" does not give "one fetch per slot"; `After` has no dedup at all (§5.3, §6.1, §7)

Dedup applies "only while a task is pending"; §7 says "re-submitting a finished key runs it again". A slot's transaction notifications arrive over hundreds of milliseconds to seconds; the confirmed fetch completes in well under that under a 5/s budget. Every notification for slot s that arrives after the fetch completed triggers a second, third, nth `getBlock` for the same slot, spending the shared 2-in-flight/5-per-second budget and producing duplicate capture records. Today's scheduler keys a slot's state for the run ("as soon as the slot is first seen"). §6.1's comment `// dedup: one fetch per slot` is false under the stated semantics, and `fin := workflow.After(finalized, slotKey(s), s, root.At(s)) // dedup` is doubly false because the dedup clause is written only on `Submit`.

**Fix.** Either a kind option `Remember()` (keys stay deduped after completion until `Forget(key)` or `Close`), with the memory cost stated, or drop the claim that the per-slot state struct disappears from the consumer. Also state what happens to `in` when a `Submit` dedups against a pending task with a different `in` (silently dropped today).

## 5. MAJOR — "Bounding root ingress bounds everything downstream" is false (§5.3, §5.6, Q2)

`MaxPending` counts "submitted-but-unfinished tasks" of *that kind*. A decode root is finished the moment its handler completes it; its `Each` children then live in `admit` where `MaxPending` blocks nobody (continuations never block). So decode's pending count drops, more roots enter, and the admit reorder buffer (head-of-line blocked per finding 1, or behind a slow bbolt commit) grows without bound. §5.6's "bounded in practice by MaxPending on the roots" is only true if a root stays pending until its whole subtree is terminal, which is exactly the per-root refcount finding 1 needs. Same structure, two bugs.

**Fix.** Define `MaxPending` on a *root* kind as a bound on roots with a non-terminal subtree, or add a workflow-wide `MaxTasks` that blocks root `Submit` only. Answer Q2 with a number: the bound is `MaxPending(decode) × max fan-out`, and fan-out is unbounded for `Each`.

## 6. MAJOR — The single scheduler goroutine plus blocking `Batcher.Add` is the deadlock the proposal warns about, one level up (§5.1, §5.9, §5.3)

§5.3 correctly says a handler that blocks on a queue while holding a worker slot deadlocks. §5.9 then says `Run` starts "one scheduler goroutine and one batch.Batcher per kind", and readiness "adds tasks to their kind's batcher", where `Add` blocks on a full queue.

**Deadlock scenario.** `admit` queue (1024) is full; scheduler goroutine is blocked in `Add(admit)`. `admit`'s one worker is in the handler and calls `t.Complete(out)`. If `Complete` (or `Gate.Advance`, or `Then`) hands its event to the scheduler goroutine over a channel, it blocks, the worker never returns, the queue never drains, the scheduler never unblocks. Same shape with `Submit` under `MaxPending` called from any handler, and with `Handle.Result` called inside a handler of a `Workers(1)` or `Pool(2)` kind (never forbidden). Even without a full deadlock, one slow kind's full queue stalls readiness propagation for *every* kind (cross-kind head-of-line blocking), and the WebSocket goroutine calling `root.Advance` stalls behind it, which stalls capture.

**Fix.** The scheduler must never block: `Complete`/`Advance`/`Then` take a mutex and append; the per-kind ready set *is* the batcher's queue (batcher pulls from the scheduler rather than the scheduler pushing into a bounded channel), or the ready queue is unbounded and only root `Submit` counts. Document "never call `Result`, `Submit` or `Gate.Wait` from inside a handler" and make the race detector tests in §9 include it.

## 7. MAJOR — The drain/abort story contradicts its own example; `--duration` as wired is an abort, not a drain (§4.2)

§4.2: "Close means drain ... This is what a `--duration` cap that expires normally needs." The example passes the same `ctx` to `Run` and to the reader. When the duration deadline fires, `reader.Records(ctx)` ends, `Add` returns `ctx.Err()`, the producer returns an error, `defer b.Close()` starts a drain, then errgroup cancels `ctx` (already expired anyway) and `Run` aborts: queued items are dropped. The documented "errgroup-shaped" wiring loses data on exactly the case it was written for. The consumer needs two contexts (ingestion ctx with the deadline, `Run` ctx with a later cleanup deadline), and the proposal must say so and show it.

Second hole in the same section: "Even on abort, Run returns only after every in-flight handler has returned" plus "a handler that must finish a side effect does so under `context.WithoutCancel`". A handler of batcher A that does `b2.Add(context.WithoutCancel(ctx), x)` after b2's `Run` has already exited (same parent ctx, full queue) blocks forever, because `Add` is specified to return `ctx.Err()` (its own ctx, not cancelled) or `ErrClosed` (Close was not called). A's `Run` then never returns and `g.Wait` hangs. `Add` must also fail with `ErrClosed` once `Run` has returned for any reason.

Third: the stop error policy says "the first handler error aborts the batcher" but never says whether the other in-flight workers' ctx is cancelled. Say it (internal ctx cancelled on stop) or `Run` waits on a handler that is not told to stop.

## 8. MAJOR — `Close` returning `*Incomplete`: wrong signature, racy, and leaks handles (§5.2, §5.9)

§5.2 declares `func (w *Workflow) Close()`; §5.9 says `Close` "returns ... the `*Incomplete{Pending []Key}` error". Presumably `Run` returns it, but then: (a) quiescence must be defined to include tasks blocked on retry backoff, on `InOrder` head-of-line, and on transitively gate-blocked deps, not only "gate-blocked"; (b) `Run` decides "quiescent" at instant t while the WebSocket goroutine calls `root.Advance` at t+ε; the newly ready tasks have no scheduler; (c) their handles never resolve, so any goroutine in `h.Result(context.Background())` leaks forever. The `Handle` of a forgotten task (question in the brief) has no defined state.

**Fix.** On `Run` return for any reason, fail every remaining task with `*Incomplete` (or `ctx.Err()`) so every handle resolves; `Advance` after that is a no-op; `Close` is non-blocking and idempotent; say what `Run` after `Close`, `Close` before `Run`, and double `Close` do.

## 9. MAJOR — `Deadline` and `RetryPolicy.Budget` are two clocks doing one job, and both make `InOrder` nondeterministic (§5.3, §5.7)

`Deadline(d)` is per task from submission; `Budget` is per kind from readiness. Neither says which error the task fails with when the clock fires while (a) the task is in the reorder buffer, (b) in a handler (which cannot be interrupted; does the eventual `Complete` win or the deadline?), (c) waiting for a `Pool` slot. A deadline that fires while a task sits in an `InOrder` buffer removes it from the buffer at a wall-clock instant, so a replay releases a different sequence, contradicting §5.6 again. §6.4's paper runner combines `InOrder` with gate-fed tasks; add one `Deadline` and its output is time-dependent.

**Fix.** Cut `Deadline`. Keep `Budget`, define `*RetryError{Attempts: 0}` for "never started" (see finding 10), and state that `InOrder` kinds must not use time-based failure.

## 10. MAJOR — `Pool` strict priority starves; backoff slot-holding unspecified; §7 contradicts §5.8

"The kind with the lowest priority number is served first; ties are FIFO" with no aging. Under sustained `confirmed` load (priority 0), `finalized` (2) never gets the pool; its tasks burn their 2-minute `Budget` waiting and fail as `RetryError` with zero attempts, which the consumer will count as "unresolved at the deadline" evidence of nothing, at the cost of never fetching finalized blocks. §7 then says "Priorities across kinds: deliberately absent", in the same document that defines them.

Also unspecified: during a group retry's backoff, does the group hold its worker slot and pool slot? If yes, a `Workers(1)` kind is frozen for the backoff and a shared `Pool(2)` loses half its capacity to a sleeping group; if no, the group must be re-queued outside the batcher, which is a second queue the batcher knows nothing about. And `WithLimiter`'s FIFO `Wait` runs *after* priority admission, so a low-priority group that got in holds an in-flight slot while it waits at the limiter.

**Fix.** Either drop `Pool` priority (Q6: yes, wrap the RPC client in the consumer; the priority semaphore is 40 lines) or define aging/weighted admission and that backoff releases both slots.

## 11. MAJOR — Ordinal rules collide and are positional (§5.6)

- `Each` "sets minor to the element's index". A second `Each` downstream of a first (`Each` of an `Each` child) overwrites minor, so two tasks can share `(major, minor)` in one `InOrder` kind; tie-break unspecified; nondeterministic.
- "Inherits the ordinal of its first dependency": swapping `Join2(k, key, a, b, …)` to `Join2(k, key, b, a, …)` silently changes replay order. Nothing in the type system marks the ordering dependency.
- `After(statuses, sigKey(sig), sig, root.At(s))` has only a gate dependency; a gate has no ordinal; the task's ordinal is undefined.
- `Order(major, minor)` lets the user pick a major that the `Submit` counter will later reuse.

**Fix.** Ordinals must be a path (`[]uint64`, root counter then each fan-out index appended), or explicit-only on `InOrder` kinds (Q3: yes, explicit). Reject `After` with no task deps on an `InOrder` kind unless `Order` is given.

## 12. MAJOR — The API as written does not compile, and the naming is careless (§5.2, §5.8)

`type Pool struct` and `func Pool(p *Pool, priority int) KindOption` in the same package is a redeclaration error. Around it: `workflow.Policy(p batch.Policy)` (func named after the type it takes), `workflow.Workers` vs `batch.WithWorkers`, `workflow.Retry(RetryPolicy)` next to `RetryPolicy.Retry`, and two unrelated `Handler` types with different signatures one import apart. A maintainer rejects this on the first read.

**Fix.** One option prefix (`With…`) in both packages; rename the `Pool` option `WithPool` or `Shared`.

## 13. MAJOR — Answer-after-return and double-answer are unspecified races (§4.6, §5.2)

"Calls left unanswered when the handler returns are failed with … `ErrUnanswered`." A handler that spawns a goroutine and returns nil races that goroutine's `Reply` against the auto-fail. `Reply` twice, `Reply` then `Fail`, `Complete` after `Fail`: none defined. `Group.Do(ctx, in)` with the caller's ctx ending while its call is inside a running group: does `Do` return `ctx.Err()` and the later `Reply` go to a dropped slot (must be a 1-buffered channel or once, or the handler blocks), or does `Do` wait? Every one of these is a `-race` failure or a goroutine leak in the first implementation.

**Fix.** First answer wins (`sync.Once`), later answers are ignored and counted in `Stats`, `Reply` never blocks, `Do` returns on its ctx and the reply is discarded. Say so in the doc comment.

## 14. MAJOR — Where does `build` run? (§5.3)

`build func(A) (In, error)` is invoked when the dependency completes. If that is on the scheduler goroutine (the natural reading), user code runs on the one goroutine everything else waits on; a `build` that decodes or allocates stalls every kind. If it runs in the completing kind's worker (inside `Complete`), the dependent's work is charged to the wrong kind's `Workers`/`Pool`. **Fix:** run `build` in the dependent kind's worker immediately before its handler, with a `build` error failing the task there. That also removes the odd "build error propagates like a `DependencyError`" rule.

## 15. MINOR — `WithOrdered` is unobservable or a no-op (§4.2, §4.5)

Groups are formed by one goroutine in release order and handed to a semaphore-bounded worker; "invoked in release order" without "completed in release order" is observable only through side effects at handler entry, and even that requires a ticket, not a channel (multiple receivers on a Go channel start in scheduler order). No consumer use case in §6 needs it. Cut it; `Workers(1)` gives the only ordering anyone can use.

## 16. MINOR — Policy edge semantics (§4.1, §4.2)

- Zero `Policy` "releases every item immediately": with `Workers(1)` and a backlog, every group is size 1 (v0.5's `MinItems=1` behavior). For a group-commit handler the useful default is "everything queued when a worker frees". State which.
- `NewGroup` with the zero policy coalesces nothing; the request/reply type's default defeats its purpose. Require or default a `MaxWait`.
- `SetPolicy` "takes effect for the next group": the current pending group under an old `MaxWait: 0` never re-arms. Say it re-evaluates or say it does not.
- `Flush` must be documented non-blocking (a `Workers(1)` handler calling `Flush` otherwise self-deadlocks); `Close` idempotence and `Run`-before/after-`Close` unspecified.
- `MaxWait` "since the group's first item" differs from v0.5 (timer from group start, reset on empty fire); fine, but say it is a change.
- Handler panics: x/sync/errgroup ≥ v0.11 recovers and re-panics from `Wait`; "not recovered" from a library-owned worker goroutine kills the process with a library frame on top. Match errgroup (Q4).

## 17. MINOR — Surface and duplication (§3, §4, §5)

Roughly 90 exported identifiers across the two packages (v0.5 has about 25) for a design whose goal 4 is "small". Two mechanisms per job:
- `batch.Group`/`Do` is a one-kind `Workflow` with `Submit`+`Result`; keep one.
- `Then`/`Join2`/`Join3`/`JoinAll` are `After` plus `Result` on already-satisfied handles (non-blocking by construction); `After` alone covers them and removes the "first dependency" ordinal question.
- `Deadline` vs `Budget` (finding 9); `WithOrdered` vs `InOrder` (finding 15); `Pool` vs `Workers` (finding 10); `Observer` vs `Stats` (an `Observer` called synchronously on the scheduler goroutine is one slow metrics push away from finding 6).
- `Error.Items any` + `ItemsOf[T]` should be `Error[T]` with `Items []T`; `errors.As` works with generic types.
- `GroupOf(ctx)` smuggles in-band data through context; put `Attempt`/`Group` on the handler argument.
- `[]*Task[In,Out]` reintroduces the per-item pointer allocation §1 lists as a v0.5 defect.
- `Kind[In,Out]` as a value with a hidden pointer: a zero `Kind{}` panics at first use with no nil check to write; `*Kind` is the honest type.
- `Consume`/`FromChan` are five-line loops; the doc says "no interfaces where a function type will do" and then ships helpers where a `for range` will do.

## 18. MINOR — Claims that are false as stated (§4.2, §5.4, §5.6, §6.1)

- "Nothing in the library ever manufactures ordering": the `Submit` counter manufactures a total order over concurrent submitters (goroutine-schedule dependent), the reorder buffer manufactures release order, `Pool` ties are FIFO by arrival, and a `Group`'s `calls` slice order (which `getSignatureStatuses` aligns by index, ruling 11 in the consumer) is queue arrival order.
- "errgroup-shaped": errgroup is `Go`+`Wait`; this is `Run`+`Close` from two goroutines, which is `http.Server`-shaped (`Serve`/`Shutdown`). Not wrong, but the analogy leads to the miswired example in finding 7.
- §6.1 "what disappears: … the retry loop, the batch assembly with its frozen lists and BatchIDs": the consumer's status batch must journal the same signature list on retry (ruling 11), and the proposal's `Group` ID plus group retry provide that only if finding 3 is resolved in favor of whole-group retry with no partial answers; otherwise the frozen list comes back in consumer code.
- §6.2 "acceptance (1), byte-identical logs, is preserved by construction rather than by care": finding 1.

---

## Verdict

Not approvable for implementation as written. Three things are unimplementable or self-contradictory, not merely underspecified: `InOrder` has no low-watermark and therefore cannot deliver the determinism the whole §6.2 mapping rests on; `Each` cannot return a sized slice without blocking; and the retry/completion rules define three incompatible outcomes for one handler error. Fix those first, in that order, because the first two share one data structure (per-root outstanding subtree count), and that structure also repairs the false "root ingress bounds everything" claim. Then make the scheduler non-blocking (finding 6), fix the drain/abort wiring and post-exit `Add` (7), and give `Close` a real quiescence-and-fail-remaining semantics (8). Cut `Deadline`, `WithOrdered`, `Pool` priority, and the `Join` family before writing code; each one removes a finding above. Resubmit with the ordinal model and the task state machine (registered → ready → in-handler → answered/failed/retrying/incomplete, with every transition's owner goroutine named) written out; without those two tables the implementation will discover these bugs under `-race` one at a time.
