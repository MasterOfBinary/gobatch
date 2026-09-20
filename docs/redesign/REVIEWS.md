# Review record for the redesign proposal

Revision 1 of [PROPOSAL.md](PROPOSAL.md) was reviewed by three independent
reviewers, each briefed differently and each working from the code, not from
the proposal's claims. Their full reports are under [reviews/](reviews/).
This file lists every finding that changed the design, what it changed, and
the findings that were rejected with the reason.

## Reviewers

| Report | Brief |
|---|---|
| [01-adversarial-r1.md](reviews/01-adversarial-r1.md) | Break the design: deadlocks, leaks, unachievable guarantees, contradictions, API bloat |
| [02-shitquant-fit-r1.md](reviews/02-shitquant-fit-r1.md) | Check every claim in section 6 against the ShitQuant code, with file:line evidence; judge adoption under the repo's own rules |
| [03-go-api-r1.md](reviews/03-go-api-r1.md) | Go idiom, generics ergonomics (verified by compiling stubs), naming, errors, options, testability, comparison with errgroup, conc, dataloader |

Verdicts on revision 1: adversarial "not approvable as written" (three
blockers); consumer fit "section 6 does not describe ShitQuant; six of seven
mapped claims wrong"; API "five changes to insist on before implementation".

## Findings that changed the design

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

## Findings rejected or deferred

| Finding | Source | Reason |
|---|---|---|
| Keep shared capacity as a standalone `Budget` primitive with `Acquire(ctx, priority)` | API 10 | Strict priority starves (adversarial 10) and the only consumer already has this in forty lines around its client. Listed as a non-goal; revisit if a second consumer needs it |
| Handles plus a synchronous observer are enough; no `Results` iterator | API 15 | Agreed; nothing added |
| Recover handler panics and fail the task to keep the process alive | archived ShitQuant design via the explorer | Rejected in favour of re-raising from `Run` as `errgroup` does; a scheduler that survives a panic cannot honour deterministic replay |
| The `workflow` package is the archived actor/hub system with generics and should not exist for ShitQuant | fit §7 | Partly accepted: the proposal now says the consumer evidence for it today is thin and the first fit is phase 6. The package remains in the proposal because the request for fan-out and dependency scheduling is the user's, and it is a library, not a re-creation inside the consumer. Open question 4 records this |
| Two packages versus one | API 17, open question 1 | Two kept; `Loader` and `Consume` stay in `batch` |
