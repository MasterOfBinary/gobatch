# Review record for the redesign proposal

[PROPOSAL.md](PROPOSAL.md) went through two review rounds with three
independent reviewers, each briefed differently and each working from the
code, not from the proposal's claims. Their full reports are under
[reviews/](reviews/). This file lists every finding that changed the design,
what it changed, and the findings that were rejected with the reason.

## Reviewers

| Report | Brief |
|---|---|
| [01-adversarial-r1.md](reviews/01-adversarial-r1.md) | Break the design: deadlocks, leaks, unachievable guarantees, contradictions, API bloat |
| [02-shitquant-fit-r1.md](reviews/02-shitquant-fit-r1.md) | Check every claim in section 6 against the ShitQuant code, with file:line evidence; judge adoption under the repo's own rules |
| [03-go-api-r1.md](reviews/03-go-api-r1.md) | Go idiom, generics ergonomics (verified by compiling stubs), naming, errors, options, testability, comparison with errgroup, conc, dataloader |
| [04-adversarial-r2.md](reviews/04-adversarial-r2.md) | Second pass on revision 2 |
| [05-shitquant-fit-r2.md](reviews/05-shitquant-fit-r2.md) | Second pass on revision 2, section 6 |
| [06-go-api-r2.md](reviews/06-go-api-r2.md) | Second pass on revision 2, with every signature compiled |

Verdicts on revision 1: adversarial "not approvable as written" (three
blockers); consumer fit "section 6 does not describe ShitQuant; six of seven
mapped claims wrong"; API "five changes to insist on before implementation".

Verdicts on revision 2: adversarial "approvable for `batch` now; not yet
for `workflow`" with two spec bugs an implementer would reproduce; consumer
fit "the architecture picture is now correct" with one design-level hole
left in 6.2; API "everything compiles; the problems are semantic", five
changes to insist on. Revision 3 addresses the round-two findings below.

## Round 1: findings that changed the design (revision 2)

| Finding | Source | Disposition in revision 2 |
|---|---|---|
| `InOrder` with ordinals inherited from the first dependency has no low-watermark: a reorder buffer cannot know that an unregistered task with a smaller ordinal may still arrive, so "byte-identical replay by construction" was false | adversarial 1, fit F1 | Implicit ordinals removed. Default release is readiness order (5.6). Explicit ordering is `Sequence` with consumer-called `Settle(n)`, and the doc states the head-of-line cost and that sequenced kinds must depend only on things that always complete |
| Ordering by first dependency inverts ShitQuant's completing-record admission order and, through the non-decreasing availability clamp, changes what `available_time` means; a block that never arrives would stall the live view forever | fit F1 | Readiness order is defined as the order of the event that completed the last dependency, which equals completing-record order when driven from one goroutine. Section 6.2 rewritten around that |
| `Each` cannot return `[]Handle[Out]` without blocking, since the count is unknown until the dependency completes | adversarial 2, API 2 | `Each`, `Then`, `Join2/3/All` removed. `After(deps, build)` is the only continuation; fan-out of unknown cardinality is a handler calling `After` per element, which is non-blocking |
| Three incompatible rules for one handler error (auto-fail unfinished, per-task retry, whole-group retry) | adversarial 3, API 11, fit F3d | One rule (5.7): unfinished remainder is re-invoked as a group with the same `Seq`; completed tasks are never re-sent; `Fail`-ed tasks retry alone; a task failure never ends the run, only `Fatal` does |
| The normalizer never fetches; its pending map is a keyed, externally completed promise, which the API had no primitive for | fit F2 | `Promise[Out]` added (5.4): `Resolve` first-wins, `Handle(key)` registers on demand, unresolved keys reported at close |
| Dedup only while pending does not give "one fetch per slot"; ShitQuant dedups for the run | adversarial 4, fit F3e | `Remember` kind setting: finished keys stay deduplicated for the run |
| "Bounding root ingress bounds everything" was false because `MaxPending` counted a kind's tasks, not a root's subtree | adversarial 5 | `WithMaxOpen` counts roots whose descendants are not all finished; per-root descendant counter; memory bound stated as `MaxOpen` times largest subtree |
| A single scheduler goroutine doing a blocking `Batcher.Add` deadlocks against handlers that call `Complete`, `Advance` or `After` | adversarial 6 | Scheduler never blocks; ready queues are unbounded and batchers pull; the handler rules in 5.8 forbid `Submit`, `Result` and `Wait` inside handlers |
| The drain/abort example used one context, so a `--duration` expiry aborted instead of draining; `Add` after `Run` returned could block forever | adversarial 7 | Two-context example in 4.2; `Add` returns `ErrClosed` after `Run` returns for any reason; abort wins over drain after `Close` |
| `Close` declared as returning nothing and as returning `*Incomplete`; quiescence and forgotten handles undefined | adversarial 8, API 3 | `Close` is non-blocking and idempotent; `Run` fails every pending task with `*IncompleteError{Pending []PendingTask}` before returning; `Resolve`/`Advance`/`After` after return are no-ops |
| `Deadline` duplicated `RetryPolicy.Budget` and made ordered release time-dependent | adversarial 9, API 8 | `Deadline` removed |
| `Pool` with strict priority starves low-priority kinds; backoff slot-holding unspecified; contradicted section 7 | adversarial 10, API 10 (kept as standalone `Budget`), fit F3f/F3g | Removed entirely and listed as a non-goal; a consumer wraps its client in a semaphore and limiter. The API reviewer's standalone `Budget` was not adopted, to keep the surface small |
| `type Pool` and `func Pool` in one package do not compile; option naming inconsistent between packages | adversarial 12, API 1 | `KindConfig` struct replaces kind options; all remaining options are `With…` |
| Double answers, answers after return, and `Do` with an ended context were unspecified races | adversarial 13 | First answer wins, later ones ignored and counted; `Do` returns `ctx.Err()` and the reply is discarded, with the caller warned it cannot know whether the handler acted |
| Where `build` runs was unspecified | adversarial 14 | In the dependent kind's worker, immediately before the handler |
| `WithOrdered` unobservable or a no-op | adversarial 15, API 7 | Removed; release-order hand-off is the documented default |
| Zero `Policy`, `SetPolicy` on a pending group, `Flush` blocking, `MaxWait` origin change | adversarial 16 | Each stated in 4.1/4.2: zero policy is greedy, `MaxItems: 1` is per-item, `SetPolicy` re-evaluates, `Flush` never blocks, `MaxWait` measured from the first item |
| `Error.Items any` plus `ItemsOf[T]` existed only to serve a non-generic `OnError` | API 4 | Both removed with `OnError`; `Run` returns `*GroupError{Seq, Attempt, Err}`; `GroupFromContext` replaces `GroupOf(ctx)` |
| `batch.Group` collides with `errgroup`/`singleflight` vocabulary while "group" also meant a released batch | API 5 | Request/reply type renamed `Loader`; `Call.Reply` became `Complete` to match `Task` |
| Interface-typed `In` does not infer through free functions with an `in In` argument (verified) | API 9 | `Submit` and `After` are methods on `*Kind`; `Kind` is a pointer type |
| Result lifetime stated two ways | API 12 | Handles own results; scheduler holds nothing after completion |
| Go 1.23 floor leaves timer tests without `testing/synctest` | API 13 | Go 1.25 |
| Error types: `Cause` field, bare `Fail` errors losing their task identity, `[]Key` in `Incomplete` | API 14 | `DependencyError`, `TaskError`, `RetryError`, `IncompleteError` with `Err` fields and `Unwrap`; `PendingTask{Kind, Key, Waiting}` |
| `FromChan` cannot honour ctx while blocked on the receive | API 15 | Removed |
| `Gate` suggests open/closed; positional `Observer` cannot grow | API 16 | `Watermark`; `func(Event)` |
| 6.1's statuses policy `MaxItems: 256` released every item immediately; one retry classifier for all kinds made skipped-slot codes terminal at confirmed commitment | fit F3a, F3b | Example rewritten with `MinWait` and per-kind classifiers, and the text states that batch assembly order still differs from today |
| The request loop's normal end is abandon-and-drain-results, which is the proposal's abort, not its drain | fit F3c | Stated in 6.1 |
| "536 lines disappear" was not honest; about 280 net lines move, onto the irreplaceable capture path | fit F4 | 6.1 now gives the review's count and recommends against adoption there |
| "Nothing changes inside the recorder" was false: group commit needs a transactional index API | fit F5 | 6.3 corrected |
| A `Loader` in front of a journal append gives the caller an uncertain outcome on ctx expiry | fit F6 | Stated on `Do` |
| Adoption order and the "smallest blast radius" claim were backwards | fit §7, misread 16 | Delivery plan step 4 is "nothing until a measurement asks; then phase 5, then phase 6" |

## Round 2: findings that changed the design (revision 3)

| Finding | Source | Disposition in revision 3 |
|---|---|---|
| Retry was dead code: a task was failed (handle resolved, dependents failed) before it was retried, so a retry could never surface a result | adversarial r2 1 | New `retrying` state: handles stay unresolved and dependents untouched until terminal failure; state table updated |
| `MaxOpen` counted nothing in the proposal's own examples: `After` tasks under a promise or watermark had no root, and handler-side `After` could not name its parent | adversarial r2 2, API r2 1 | `Submit(ctx, key, in, deps ...Dep)` is the only root entry and the only thing `MaxOpen` counts; `After` requires a task-handle dep and joins the root of every task-handle dep; `Task.Handle()` is the handler-side parent; a finished root reopens |
| Re-injecting a retried group with its original `Seq` is not expressible through `batch.Batcher.Add`, so "one Batcher per kind" was fiction | adversarial r2 3 | Stated: kinds and `Batcher` share an internal release engine; `workflow` does not drive `Batcher`'s public API; `Group.Partial` added so a handler knows a remainder from an original |
| The sequence rule never released a position that failed before release, so everything behind it waited forever | adversarial r2 4 | Rule is "released or terminally failed"; `At(n)` panics on reuse or `n <= settled`; ties by registration order; retries bypass; "released" defined as a scheduler-side event |
| Readiness order omitted two readying events and never said what "happened" means across goroutines | adversarial r2 5 | 5.1 lists every readying event and defines order as scheduler-lock acquisition order with positions assigned under the lock; watermark waiters ready in registration order; determinism is of release order, not group boundaries |
| Promises and `Remember` retained every value with no eviction | adversarial r2 6, API r2 3 | `Promise.Forget`, `Kind.Forget`, unreferenced counts in `Stats`, bound stated as the consumer's |
| Policy timers were defined on "queued" while kinds pull from a ready queue | adversarial r2 7 | Kind timers run from readiness; slice order is queue order; a group whose builds all failed is skipped |
| `Fatal` interplay with the classifier and the group's tasks was unspecified | adversarial r2 8, API r2 4 | `Fatal` bypasses the classifier, fails the group's unfinished tasks with the unwrapped error, is honoured from handler, `build` and `Fail`; `ErrFatal` exported; `GroupError.Attempt` removed |
| `Close` did not quiesce a consumer that kept calling `After` and `Resolve`; the sweep raced a late registration | adversarial r2 9 | After `Close`, `After` returns a failed handle and promise completion returns false; the sweep and the closed flag are one critical section |
| `Watermark.At` waiters created an AB/BA lock pair with the scheduler | adversarial r2 10 | Waiters registered and copied outside the scheduler lock; lock order documented |
| Duplicate `Submit` must not wait on `MaxOpen`; a never-completed promise keeps a root open | adversarial r2 11 | Dedup precedes the wait; the consumer's escape is `Fail` on the key |
| "A late answer races the auto-fail and loses" was false as a race | adversarial r2 12 | Worker closes the task under its lock before failing unanswered ones |
| Two attempt counters and a budget that could be exceeded by one attempt | adversarial r2 13, API r2 5 | `Task.Attempt` is the task's, `Group.Attempt` the group's, observer reports the task's; budget from first readiness, checked before each re-invocation |
| `Kind.Ordinal(n)` returning a view was stateful and per-call data on the kind | API r2 2 | Replaced by `Sequence.At(n) Dep`; `KindConfig.Sequence` removed |
| Three completion vocabularies (`Complete`, `Reply`, `Resolve`/`Reject`) | API r2 3 | `Complete`/`Fail` returning `bool` on `Task`, `Call` and `Promise` |
| Zero-handle and `Result`-inside-handler contradictions; `DependencyError` wrapping `IncompleteError`; `Stats` type unshown; `PendingTask.Waiting` singular | API r2 5, adversarial r2 14 | Each stated in 5.3, 5.8, 5.9, 5.11 |
| Observer inside the scheduler lock would self-deadlock on `Complete` | adversarial r2 14 | Called outside the lock; `Event.Seq` carries transition order |
| 6.2 "adopt the join alone" split admission across the kind's worker and the reader goroutine, racing for the recorder's sequence | fit r2 R1 | 6.2 now says expressible, not adoptable in part, and not worth adopting whole |
| `block_conflict` over-counted; the promise value lacked the record's receipt and provenance; "blocks awaiting swaps" not reproducible | fit r2 R2 | Snippet counts only on a differing hash; promise holds the arrival triple; `Stats.Promises` unreferenced count |
| 6.1 claimed two in flight per kind and silently dropped cross-kind priority; batch budget and per-slot signature dedup differ | fit r2 R3 | 6.1 states each difference and that cap, limiter and priority move into the RPC client |
| Section 6 misattributed the commit measurement; 6.4 invented a phase 6 shape | fit r2 R4 | Corrected to the capture journal's measurement; 6.4 defers to the phase 6 design |
| Ship `batch` as 0.6 and `workflow` as 0.7 | API r2 6 | Adopted in the delivery plan |

## Findings rejected or deferred

| Finding | Source | Reason |
|---|---|---|
| Keep shared capacity as a standalone `Budget` primitive with `Acquire(ctx, priority)` | API 10 | Strict priority starves (adversarial 10) and the only consumer already has this in forty lines around its client. Listed as a non-goal; revisit if a second consumer needs it |
| Handles plus a synchronous observer are enough; no `Results` iterator | API 15 | Agreed; nothing added |
| Recover handler panics and fail the task to keep the process alive | archived ShitQuant design via the explorer | Rejected in favour of re-raising from `Run` as `errgroup` does; a scheduler that survives a panic cannot honour deterministic replay |
| The `workflow` package is the archived actor/hub system with generics and should not exist for ShitQuant | fit §7 | Partly accepted: the proposal now says the consumer evidence for it today is thin and the first fit is phase 6. The package remains in the proposal because the request for fan-out and dependency scheduling is the user's, and it is a library, not a re-creation inside the consumer. Open question 4 records this |
| Two packages versus one | API 17, open question 1 | Two kept; `Loader` and `Consume` stay in `batch` |
| Implicit `Settle` when the next ordinal is submitted | open question 1 of revision 2 | Rejected by API r2: a record can yield ordinals in any order, so "n+1 registered" does not mean "n settled" |
| Bind `Watermark` to a workflow for symmetry with `Promise` | adversarial r2 10 | Kept standalone so a journal follower can use `Wait` without a workflow; the lock order is documented instead |
| `RememberKeys` as a cheaper mode without results | API r2 smaller | Deferred to a measurement; recorded as open question 3 |
