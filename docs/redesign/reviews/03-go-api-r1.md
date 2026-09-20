# Review of docs/redesign/PROPOSAL.md (Go API design)

Scope: the proposed `batch` and `workflow` packages, read against v0.5
(`batch/batch.go`, `config.go`, `helpers.go`, `processor/`, `source/`). Claims
about type inference and redeclaration below were checked by compiling stub
signatures with go1.24.7; the scratch module is in
`scratchpad/infer/` next to this file.

## Findings, ranked by impact

### 1. `workflow.Pool` is declared twice (compile error)

Section 5.2 lists `func Pool(p *Pool, priority int) KindOption` and 5.8
declares `type Pool struct`. One package cannot hold both:

```
poolclash/p.go:6:6: Pool redeclared in this block
```

This falls out of naming every KindOption after the thing it sets
(`Policy`, `Retry`, `Pool`) while also exporting those things as types in the
same or a sibling package. Fix by replacing KindOption with a config struct
(finding 6), which removes the whole family of name clashes at once. If
functional options are kept, prefix them (`WithPool`, `WithRetry`) as `batch`
already does; the two packages currently use opposite conventions
(`batch.WithPolicy` vs `workflow.Policy`).

### 2. `Each` cannot return `[]Handle[Out]` synchronously

```go
func Each[A, In, Out any](k Kind[In, Out], dep Handle[[]A], key func(A) Key, build func(A) (In, error)) []Handle[Out]
```

The number of dependents is the length of `dep`'s result, which is unknown
until `dep` completes. `Each` cannot return a slice of handles without
blocking, and 5.3 says continuations never block. Section 6.2 ranges over the
result inside per-record ingest, so the example would deadlock or return
`nil`. Either shape below is honest:

```go
// Each queues one dependent per element of dep's result, keyed by key(elem).
// The returned handle completes with every dependent's result in element
// order, or fails with the first dependent's error.
func Each[A, In, Out any](k *Kind[In, Out], dep Handle[[]A], key func(A) Key, build func(A) (In, error), opts ...TaskOption) Handle[[]Out]
```

or, if per-element results are never consumed (the admit sink), drop the
return and let `Observer`/`Stats` report. The gather form is the one that
composes (`Then` on the gathered handle is the "persist after all swaps of
this transaction were admitted" step).

### 3. `Close()` is defined two ways

5.2: `func (w *Workflow) Close()`. 5.9: "`Close` returns once the only
remaining tasks are gate-blocked and reports them in the returned
`*Incomplete{Pending []Key}` error." Those contradict each other, and the
second contradicts `batch.Close`, which is a non-blocking signal.

The right shape is the one `batch` has: `Close()` is the channel-close
analogue of `Add` (no more sends; the receiver drains), it takes no ctx
because the deadline is `Run`'s ctx, and it returns nothing because `Run`
returns the outcome. That is consistent with the errgroup composition and I
would keep it. `net/http.Server` needs `Shutdown(ctx) error` only because it
has no `Run` that the caller is already waiting on; here `Run` is that wait.
So:

```go
func (w *Workflow) Close()                      // idempotent; safe before Run; roots after Close -> ErrClosed
func (w *Workflow) Run(ctx context.Context) error // nil | ctx.Err() | handler error | *IncompleteError
```

Also state, in one sentence each: `Close` twice is a no-op; `Add`/`Submit`
after `Close` return `ErrClosed`; ctx cancellation after `Close` still
aborts (abort wins over drain); `Run` twice panics (the proposal already
panics for `Define` after `Run`, so be consistent and say so).

### 4. `*Error{Items any}` + `ItemsOf[T]` exist only to serve a non-generic `OnError`

`OnError(func(*Error) error) Option` cannot be typed because `Option` is
shared by `Batcher[T]` and `Group[In,Out]`, so `Items` became `any`. That is
a smell, and the option itself is unnecessary: the handler already has
`ctx` and `items`, so "log and continue" or "dead-letter and continue" is
`return nil` after the handler does it. A user who wants a reusable policy
writes a three-line closure around the handler. Delete `OnError`, `Error.Items`
and `ItemsOf`. What `Run` should return on handler failure is:

```go
// GroupError is returned by Run when a handler fails and the batcher stops.
type GroupError struct {
	Group   uint64 // release sequence number
	Attempt int
	Err     error
}
func (e *GroupError) Error() string
func (e *GroupError) Unwrap() error
```

`Group uint64` and `Attempt` are also what the handler wants from the
context (5.7's `GroupOf(ctx)`), so define one struct in `batch` and use it
in both places:

```go
type Group struct{ Seq uint64; Attempt int }
func GroupFromContext(ctx context.Context) (Group, bool)
```

which requires finding 5.

### 5. `batch.Group` is the wrong name for request/reply batching

The proposal uses "group" throughout to mean a released batch (`Error.Group`,
`GroupOf(ctx)`, "same `Group` ID", `GroupHandler`), and separately names the
request/reply type `Group`. In Go, `Group` already means a set of goroutines
or calls sharing a lifecycle (`sync.WaitGroup`, `errgroup.Group`,
`singleflight.Group`); `singleflight.Group.Do` in particular has the same
method name and different semantics (dedup, not batching). Rename to the
prior-art name for exactly this pattern:

```go
type Loader[In, Out any] struct{ ... }   // graph-gophers/dataloader, Facebook DataLoader
func NewLoader[In, Out any](h LoadHandler[In, Out], opts ...Option) *Loader[In, Out]
func (l *Loader[In, Out]) Do(ctx context.Context, in In) (Out, error)
```

If "Loader" reads as read-only, `Coalescer` is the honest alternative. Either
frees `Group` for the released-batch concept. Also expose `Flush`, `SetPolicy`
and `Stats` on it; the proposal drops them for no reason.

### 6. Five option types is too many; Kind wants a struct

`batch.Option`, `workflow.Option`, `KindOption`, `TaskOption`, `PoolOption`.
Kind's six options (`Policy`, `Workers`, `MaxPending`, `InOrder`, `Retry`,
`Pool`) are all data with meaningful zero values, which is the case where
the standard library uses a struct (`http.Server`, `tls.Config`,
`sql.DB`'s setters aside). A struct is also what the snapshot line in 5.10
wants to print, and it is what makes finding 1 impossible:

```go
type KindConfig struct {
	Policy     batch.Policy
	Workers    int          // 0 means 1
	MaxPending int          // 0 means unbounded
	InOrder    bool
	Retry      *RetryPolicy // nil means no retry
	Budget     *Budget      // shared capacity, see finding 10
	Priority   int          // within Budget; lower first
}
func Define[In, Out any](w *Workflow, name string, h Handler[In, Out], cfg KindConfig) *Kind[In, Out]
```

`workflow.Option` has one member (`WithObserver`); make it a field of a
`Config` passed to `New`, or a method `w.Observe(func(Event))`. `TaskOption`
shrinks to `Order` only (finding 8); keep it as a variadic so `Then`/`Join`
stay short. `PoolOption` has one member; make `NewBudget(inFlight int, l Limiter)`
with `nil` meaning unpaced. `batch.Option` is fine at three members
(`WithPolicy`, `WithWorkers`, `WithQueue`), after finding 7.

### 7. `WithOrdered()` in `batch` is either free or meaningless

"Handlers are invoked in release order" is a property of a single dispatcher
goroutine acquiring a worker semaphore in FIFO order; every reasonable
implementation has it without an option. With `Workers(n)` it guarantees
nothing observable (completion order is not ordered), and it is untestable
as stated. Make it the documented default and delete the option. It also
removes the confusion between `batch.WithOrdered` and `workflow.InOrder`,
which sound alike and mean different things.

### 8. `Deadline(d time.Duration)` duplicates `RetryPolicy.Budget` and is misnamed

`context` uses `Deadline` for a `time.Time` and `Timeout` for a
`time.Duration`. More importantly the two knobs overlap: `Deadline` is "wait
for readiness plus retries, from submission", `Budget` is "retries, from
readiness". A slot's readiness wait is bounded by the consumer's gate
policy, not the library. Drop `Deadline`, keep `Budget`. `TaskOption` is then
`Order` alone, and `Submit` should accept it too (see finding 9): with
concurrent submitters the root counter is nondeterministic, which defeats
the replay argument in 5.6. Answer to open question 3: ordinals should be
explicit on `InOrder` kinds' roots and inherited for continuations; a rule
"root major = submit counter" only holds for a single-goroutine submitter,
so say that in the doc of `InOrder`.

### 9. Inference works everywhere except interface-typed `In`; use methods for the `In`-only entry points

Verified call sites (all infer with no type arguments spelled):

```go
b := batch.New(persist)                                      // T from named func
b := batch.New(func(ctx context.Context, items []string) error { ... })
g := batch.NewGroup(fetchStatuses)                           // In, Out from func
decode := workflow.Define(w, "decode", decodeTx, ...)        // In=Rec, Out=[]Swap
swaps, err := workflow.Submit(ctx, decode, "r1", rec)
blk, _   := workflow.Submit(ctx, confirmed, "s8", 8)         // untyped const OK
h := workflow.Then(admit, "t1", blk, func(b Block) (Admit, error) { ... })
h := workflow.Join2(admit, "j1", swaps, blk, func(s []Swap, b Block) (Admit, error) { ... })
h := workflow.JoinAll(admit, "ja", []workflow.Handle[Block]{blk}, func(bs []Block) (Admit, error) { ... })
hs := workflow.Each(admit, swaps, func(s Swap) workflow.Key { ... }, func(s Swap) (Admit, error) { ... })
h := workflow.After(confirmed, "s7f", rec.Slot, root.At(rec.Slot), blk)   // Handle satisfies Dep
```

The one that fails, and it is exactly the ShitQuant admit case (`In` is an
interface, the argument is a concrete fact type):

```
use.go:61: in call to workflow.Submit, type swapAdmit of swapAdmit{} does not match inferred type Admit for In
```

Because `in In` is a second inference site, unification of `Admit` with
`swapAdmit` fails; the user must write `Admit(swapAdmit{})`. The same
applies to `After`'s `in In`. A method on the kind fixes it, since the
receiver fixes `In` and the argument is checked for assignability:

```go
func (k *Kind[In, Out]) Submit(ctx context.Context, key Key, in In, opts ...TaskOption) (Handle[Out], error)
func (k *Kind[In, Out]) After(key Key, in In, deps []Dep, opts ...TaskOption) Handle[Out]
```

(verified: `admit.Submit(ctx, "a1", swapAdmit{})` compiles). Keep free
functions only where a new type parameter is introduced (`Then`, `Join2`,
`JoinAll`, `Each`), because methods cannot have their own type parameters.
`Then`'s `build` literal must still spell `(Admit, error)` as its return
type; a literal returning `(swapAdmit, error)` fails with a clear message.
Note `After`'s `deps ...Dep` blocks adding `opts`; make it a slice.

`Kind[In,Out]` as a comparable value wrapping an unexported pointer works
(verified as a map key) but buys nothing over `*Kind`, and a zero `Kind` is a
nil-pointer panic at first use with a worse message. Use `*Kind` like
`*rate.Limiter` and `*semaphore.Weighted`. `Handle[Out]` as a value with a
value receiver implementing an unexported interface method works (verified)
and is fine, because handles are copied around; document that the zero
`Handle` is never done. `*Task[In,Out]` by pointer with `Complete`/`Fail` is
right for the "unfinished tasks are failed when the handler returns" rule;
state that a second `Complete`/`Fail` is a no-op (first wins) rather than a
panic, because group retry (finding 12) makes a second call reachable.

### 10. `Pool` is a good primitive hidden inside a kind option; make it standalone

Answer to open question 6: keep it, but as a primitive any handler can use,
so it is testable without a workflow and usable by the RPC client directly:

```go
// Budget bounds in-flight work across its users and optionally paces it.
// Acquire blocks until a slot is free and the Limiter admits; among waiters,
// the lowest priority number is served first, ties FIFO.
type Budget struct{ ... }
func NewBudget(inFlight int, l Limiter) *Budget   // l nil means unpaced
func (b *Budget) Acquire(ctx context.Context, priority int) (release func(), err error)
```

The kind config then just references it (`Budget`, `Priority` fields). The
name avoids `sync.Pool`, `conc/pool` and every connection-pool library, and
it is the word the proposal itself uses ("A Pool is that budget").

### 11. Handler error precedence is unspecified

5.7 says a handler error re-invokes the group; 5.9 says the first handler
error "whose kind has the stop policy" ends `Run`; there is no kind-level
stop/continue setting anywhere. Write the rule once, on `Handler`:

> A handler error fails every unfinished task in the group with that error.
> If the kind has a RetryPolicy and Retry(err) is true, the unfinished tasks
> are re-invoked as one group with the same Group.Seq and Attempt+1, up to
> Attempts and Budget. Otherwise Run returns the error and the workflow aborts.

Answer to open question 7: the retried group is the unfinished remainder.
When nothing was completed it is byte-identical, which is the status-batch
case; when something was, re-sending it would double-journal. The stable
`Group.Seq` gives the consumer the "same request" key either way.

### 12. Result lifetime is stated two ways

5.3: "A dependency's result is held only until every dependent registered
at that point has consumed it". 5.5: dependents "can be added later, even
after completion (they run immediately)". The second is the useful one and
it is what a value handle implies: the handle owns the result; the
scheduler forgets the task at completion and holds nothing. Say that and
delete the first sentence.

### 13. Go floor: 1.25, not 1.23, and for a testing reason

The API needs 1.23 (`iter`). Tests for `MinWait`/`MaxWait`, `Backoff`,
`Budget` and gate waits are timer tests, and `testing/synctest` makes them
exact and fast; it is `GOEXPERIMENT`-gated in 1.24 (verified on this
toolchain: "This package only exists when using Go compiled with
GOEXPERIMENT=synctest") and stable as `synctest.Test` in 1.25. CI already
tests 1.25 and 1.26 (CHANGELOG 0.5.0) and ShitQuant is on 1.26. Set
`go 1.25` and design the tests around `synctest.Test` instead of a fake
clock interface. That removes a `clock` seam from the public surface and
keeps timers as plain `time.Timer`.

### 14. Errors: make the structured ones conventional, keep the sentinels

- `*DependencyError{Key, Cause}` -> `{Kind string; Key Key; Err error}` with
  `Unwrap() error`. The field is `Err` in `os.PathError`, `url.Error`,
  `net.OpError`; `Cause` is the pkg/errors word.
- Task failures surfaced by `Handle.Result` should be wrapped as
  `*TaskError{Kind, Key, Err}` so a consumer three joins downstream can
  still name the task; today `Fail(err)` returns `err` bare and only
  `DependencyError` carries a key.
- `*RetryError{Attempts, Err}` with `Unwrap` is fine; `errors.Is(err, cause)`
  still works through it.
- `*Incomplete` -> `*IncompleteError{Pending []PendingTask}` where
  `PendingTask{Kind string; Key Key; Waiting Dep}`; a bare `[]Key` cannot say
  which kind or which gate.
- Sentinels `ErrClosed`, `ErrUnanswered` are right. `workflow` should return
  `batch.ErrClosed`, not a second sentinel: `var ErrClosed = batch.ErrClosed`.
- Add `ErrRunning` or panic for `Run` twice; pick the one the doc states.

### 15. Iterators

`iter.Seq2[T, error]` is the community contract for fallible streams and is
the right source type; it composes with `range`, needs no goroutine and
cannot return a nil channel. `Consume` is four lines; exporting it is
harmless. `FromChan` does not earn its place: `for v := range ch { b.Add(ctx, v) }`
is shorter than `Consume(ctx, b, FromChan(ch))`, and the adapter cannot
honour ctx while blocked on the receive, which a doc reader will assume it
does. Drop it. A `Batcher` has no results, so no result iterator; `Loader`
returns per call. A `Workflow.Results` iterator would reintroduce the
must-drain side channel that section 1 removes; handles plus the Observer
cover it. One thing `Consume` must say: it cannot interrupt `seq` itself,
only the `Add` between yields.

### 16. Naming, smaller items

- `Kind`: unusual but not wrong; `Step` reads better in graph vocabulary and
  avoids the `reflect.Kind` echo. Not insisting.
- `Gate`: suggests open/closed; the primitive is a monotone watermark with
  waiters, and 5.4 calls it that. `Watermark` with `Advance/Value/Wait/At`.
- `Then`/`Join2`/`Join3`/`JoinAll`: promise vocabulary, and `Join` in Go means
  concatenate (`errors.Join`, `strings.Join`). Tolerable for `Then` and
  `JoinAll`; drop `Join3`. With deps guaranteed complete when the task is
  built, N-ary fan-in is `After` plus `Result` on handles that return
  immediately; document that pattern instead of adding arity-suffixed
  functions.
- `Call.Reply`/`Task.Complete`: same operation, two verbs. Pick one
  (`Complete`/`Fail` on both).
- `MinWait`/`MaxWait`: good; better than `MinTime`/`MaxTime`. Keep.
- `Observer` interface with four positional args: make it
  `type Event struct{ Kind string; Key Key; Type EventType; Attempt int; Err error }`
  and `func(Event)`; a struct can grow, an argument list cannot.
- `Limiter interface{ Wait(context.Context) error }` matches `*rate.Limiter`;
  good.
- `Stats()` returning a struct snapshot matches `sql.DBStats`; good. Add a
  per-kind `Waiting` count (on Budget) because the priority test needs it as
  a sync point.

### 17. Package layout

Keep `batch` and `workflow` as subpackages with an empty root. `gobatch.New`
reads worse than `batch.New`, and `errgroup`/`semaphore`/`singleflight` set
the precedent of a package named for its one concept. The cost of `batch` as
an import name is that callers cannot name a local `batch` (v0.5's own
`waitForItems` does exactly that); it is the same cost as `url` and `path`
and acceptable. No `/v2` module path: the module is v0 and the CHANGELOG
already breaks on minor versions; a v2 path would freeze the wrong thing.
Two packages is the right cut (open question 1); `Loader` and `Consume` are
both under fifty lines and belong with the batcher they wrap.

### 18. Lifecycle details to state in godoc

Each in one sentence, currently missing:
- `Add` before `Run`: queues up to `WithQueue` then blocks until `Run` starts.
- `Add` when both ctx is done and the batcher is closed: `ctx.Err()` wins.
- `Flush` is non-blocking; it releases whatever is pending now.
- `Loader.Do` when the caller's ctx ends first: returns `ctx.Err()`; the call
  stays in its group and the handler's reply is discarded.
- `Then`/`Join`/`After` on a kind from a different workflow than the
  dependency: panic.
- All submission functions dedup by `(kind, key)` while pending, share the
  handle, and keep the first `in`; a completed key resubmitted runs again,
  and the window between completion and resubmission is inherent.
- Panics: leave Go's default (open question 4). A scheduler that survives a
  handler panic cannot honour "byte-identical replay"; conc's re-panic from
  `Wait` is friendlier for stacks but changes nothing about process survival.

## Comparison with errgroup, conc, dataloader

- `errgroup.Group.SetLimit(n)` plus `Go` is the worker bound; a `Batcher` is
  a dispatcher in front of an errgroup and can be implemented on it. The
  proposal's `Run`/`Close`/ctx split composes with errgroup correctly; the
  example in 4.2 is right, and the abort-wins rule makes producer failure
  cancel the batcher through the group ctx as expected.
- `conc/stream` is an ordered-completion pipeline (callbacks run in
  submission order) with a bounded reorder buffer. The proposal's
  `workflow.InOrder` is release-order, which is the stronger property for a
  serialized admission point, but its buffer is unbounded; conc bounds it.
  State the bound (`MaxPending` on roots) as the only bound. conc's
  `panics.Catcher` shows the propagate-to-`Wait` alternative for 18.
- `dataloader.NewBatchedLoader(fn, WithWait, WithBatchCapacity)` with
  `Load(ctx, key) Thunk` is `Loader` exactly, plus a cache and in-batch key
  dedup. The proposal's per-call `Reply` is better than dataloader's
  positional `[]*Result` (misalignment is a silent bug there; `ErrUnanswered`
  catches it here). It reinvents nothing worse; it just needs the name.
- `singleflight` is what `Submit`'s dedup-while-pending is; the proposal's
  version is keyed per kind and returns a future, which is the right
  generalization. Naming the batching type `Group` with a `Do` method is the
  one place it borrows a name without the semantics.

## Testability

- Timers (`MinWait`, `MaxWait`, `Backoff`, `Budget`, `Gate.Wait` with ctx):
  `synctest.Test` on Go 1.25; no clock seam in the API.
- `InOrder`: deterministic without timers. Handlers block on per-task
  channels; the test completes dependencies in a chosen order and asserts
  the release order from the Observer's `Ready`/`Started` events.
- Budget priority: fill the budget with a blocking handler, submit one task
  per kind, wait until `Stats().Kinds[k].Waiting` shows all waiters
  registered, release one slot, assert which kind started. The `Waiting`
  counter is the seam; without it the test races the scheduler goroutine.
- Drain vs abort: a handler that blocks until the test cancels ctx, then
  asserts `Run` returned only after the handler returned.
- `ErrUnanswered`: a handler that replies to all but one call.

## The five changes I would insist on before implementation

1. Fix the `Pool` type/function redeclaration by moving Kind configuration
   to a `KindConfig` struct (findings 1, 6) and renaming the shared-capacity
   type to `Budget` with a standalone `Acquire` (finding 10).
2. Change `Each` to return `Handle[[]Out]` (or nothing); the `[]Handle[Out]`
   signature cannot be implemented without blocking (finding 2).
3. Make `Submit` and `After` methods on `*Kind` so interface-typed `In`
   infers, and change `Kind` to a pointer type (finding 9).
4. Delete `OnError`, `Error.Items`, `ItemsOf` and `WithOrdered`; return a
   `*GroupError{Seq, Attempt, Err}` from `Run` and expose `Group` via
   `GroupFromContext` (findings 4, 5, 7). Rename request/reply `Group` to
   `Loader`.
5. Resolve the three contradictions in prose before code: `Workflow.Close`
   returns nothing and `Run` returns `*IncompleteError` (finding 3); handler
   error precedence and retried-group contents (finding 11); result lifetime
   (finding 12). Set the Go floor to 1.25 so the timer tests use `synctest`
   (finding 13).
