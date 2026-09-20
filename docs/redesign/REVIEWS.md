# Review record

[PROPOSAL.md](PROPOSAL.md) went through two review rounds. Full
reports: [reviews/](reviews/).

## Reviewers

| Report | Brief | Verdict |
|---|---|---|
| [01-adversarial-r1.md](reviews/01-adversarial-r1.md) | Break rev 1 | Not approvable |
| [02-consumer-fit-r1.md](reviews/02-consumer-fit-r1.md) | Official issues + consumers | Placement yes, contracts no |
| [03-go-api-r1.md](reviews/03-go-api-r1.md) | Signatures, inference, names | Changes required |
| [04-adversarial-r2.md](reviews/04-adversarial-r2.md) | Break rev 2 | Scheduler and shutdown still open |
| [05-consumer-fit-r2.md](reviews/05-consumer-fit-r2.md) | Re-check issues | Conditional pass; lifecycle not closed |

## Revision 2: findings that changed the design

| Finding | Source | Disposition |
|---|---|---|
| Formation unspecified once workers exist | adversarial 1, consumer #73 | 5.2 state machine: inbound ≤ n, released ≤ W, handlers ≤ W, no third buffer |
| Same Policy, two grouping laws | adversarial 2 | 5.3: Batcher greedy drain vs Loader light-load groups of one |
| `Do`/`Add` before `Run` deadlocks; 6.5 never starts Loaders | adversarial 3 | `Add` before `Run` does not wait for `Run`; `Do` before `Run` is `ErrNotRunning`; 6.5 starts `go loader.Run(procCtx)` |
| Shared Loader ctx vs run ctx | adversarial 4 | `Loader.Run` ctx is process-scoped and must outlive every `Do` |
| Three stop rules; errgroup sample self-aborts | adversarial 5 | 5.6 table; cancel wins over Close; two-context sample does not use `errgroup.WithContext` |
| Re-entry into the same worker | adversarial 6 | `ErrReentry` on Batcher, Loader, and Runner |
| Shutdown budget leaks waiters | adversarial 7, consumer #71/#95 | Budget default 0; on expiry Loader settles waiters; `Wait` joins leftover handlers |
| Call is not a state machine | adversarial 8 | `queued \| running \| settled`; no reuse; `Fail(nil)` = `ErrUnanswered` |
| Flow dispatch / `If` / `Result` unimplementable | adversarial 9 | Dispatch predicate written; `Condition *bool`; `GraphError` defined; ready nodes wait for a slot |
| Shared mutable envelope (`#97`/`#98`) | consumer 1–2, API 1 | Immutable `In` + owned `any` results + `View` + explicit join node |
| Unmeasured persist linger in the example | consumer 2 | Persist `MaxItems: 1` |
| `Run` panics if called twice | adversarial 10, consumer 7, API 5 | `ErrUsed` |
| Silent `MinItems` rewrite; `SetPolicy` hole | adversarial 11, consumer 6 | Zero means default 1; negatives rejected; Loader `SetPolicy` uses `NewLoader` rules |
| Default 30s handler timeout | all three | Default 0 |
| `flow` imports `batch` via shared types | adversarial 13, API 6 | Duplicated `Event` / `ErrClosed` / `Stats` |
| Close-drain / “started” undefined | adversarial 14 | Started = handler invoked; Close keeps forming until empty |
| `batch.Group` vs `errgroup` | API 3 | Renamed `Info` / `InfoFromContext`; dropped `Attempt`/`Partial` |
| Helpers disagree with Graph | API 4 | Deleted `Sequence`/`Parallel`; `Map` → `ForEach`; ForEach does not cancel the caller's ctx |
| errgroup panic attribution | API 9 | Batcher does not recover; flow recovers to `*PanicError` |
| `#100` linger × node bound | consumer 9 | Stated as required: blocked `Do` occupies a node slot |
| File-level over-claim | consumer 10 | 8.1 is path-level only |
| Structured errors incomplete | API 2 | `Error`/`Unwrap` on every struct; `StatusUnknown` is zero |
| `flow.Run` as a method | API 7 | Package function; open question closed |
| Observer re-entry / Flush blocks | adversarial 18, API 6 | Observer on `Run` goroutine; must not call back |

## Kept (reviewers agreed)

Drain ≠ abort. No implicit key coalescing. Missing answer ≠ zero `Out`.
Caller `Do` cancel ≠ group cancel. `flow` does not own Loaders. Finite
DAG, no Promise/Watermark/retry. Policy clocks from the first item.
Max < min is rejected. Do not put this library on capture / Helius /
normalizer maps / recorder sequencing. No shims. Go 1.25.

## Revision 3: findings that changed the design

| Finding | Source | Disposition |
|---|---|---|
| Two schedulers (cut only when worker free vs cut≠dispatch) | adversarial r2 1 | 5.3 restated on the 5.2 machine: cut is independent of a free worker |
| `Add` abort returns nil then drops | adversarial r2 2 | Blocked `Add` sees abort as `Run`'s `ctx.Err()`; `nil` means accepted |
| `Do` during drain refills inbound | adversarial r2 3 | `Do` after `Close` is always `ErrClosed` |
| Budgeted `flow.Run` vs immutable `Result` | adversarial r2 4 | Expiry fills `Result` with Canceled; late returns ignored |
| Budget clock unspecified | adversarial r2 5 | Starts on abort, or when Close has nothing left to dispatch |
| `SetPolicy` cuts from the wrong goroutine | adversarial r2 6 | Stores + wakes `Run`; only `Run` cuts |
| 6.5 `ErrNotRunning` race | adversarial r2 7 | `Started()`; example waits before `Do` |
| `Do` full-queue unspecified | adversarial r2 8 | Same wait rules as `Add` |
| Negative durations silently mean “none” | adversarial r2 9, consumer r2 5 | `< 0` is `ErrInvalidPolicy`; `== 0` means none / default |
| Failure + cancel, two `err` values | adversarial r2 10 | Failed wins; `err` is `*GraphError` |
| `Wait` only after budgeted return | adversarial r2 11, consumer r2 1 | `Wait` always joins `Run` + handlers |
| 6.5 Close ∪ cancel | consumer r2 1 | Defers: Close+Wait first, `stop` last; abort recipe is cancel first |
| Budget expiry does not settle `Do` | consumer r2 2, adversarial r2 Adv7 | Expiry settles every still-unsettled `Do` with `*ShutdownError` |
| Default infinite budget vs `#95` | consumer r2 3 | Mechanism required; default 0 is a house-rule exception; redeploy sets it |
| `#98` Close does not cancel | consumer r2 4 | `Runner.Close` stops admission, then grace, then cancels node children |
| `#71` finite handler timeout | consumer r2 6 | Option exists; default 0 documented as the exception |
| `View` heap sharing | consumer r2 7 | Written: same `any`, mutating slices/maps/pointers is a user bug |
| Goroutine per waiting node | consumer r2 8 | One dispatcher per run; ready list, not a goroutine per node |
| `GraphError.Unwraps` | adversarial r2 12 | Removed; `Error` uses `errors.Join` |

## Still open after revision 3

Honesty tag number. Optional later `WithCoalesce`. `View.Get` type
assertions vs generated joins. Whether a production default shutdown
budget should be non-zero (kept 0 so a deadline never invents failure).
