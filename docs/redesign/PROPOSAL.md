# GoBatch redesign: batching, request coalescing, and in-process flow

Status: revision 3 (2026-09-20). Revision 1 was not approvable. Revision 2
closed the data-model and naming blockers; a second review round found
remaining scheduler and shutdown holes. Reports are under
[reviews/](reviews/). [REVIEWS.md](REVIEWS.md) lists every finding that
changed this text.

This is a from-scratch redesign of GoBatch with workflow-style fan-out.
It is not the dynamic keyed-task scheduler on
`origin/claude/library-redesign-workflow-9kjqu1`. Official issues `#97`–`#100`
already chose a finite in-process graph named `flow/`.

Private consumer repos were not readable here. ShitQuant mapping uses
public issues plus a prior review of that tree, at path granularity only.

```
             ┌─────────────┐
             │ Flow runner │   one input, finite DAG, bounded
             └──────┬──────┘
                    │  owned results, explicit join
       ┌────────────┼────────────┐
       ▼            ▼            ▼
   Decode        Metadata      Prices     shared Loaders (process lifetime)
       │            │            │
       └────────────┼────────────┘
                    ▼
              evidence + persist
```

## 1. What the current library gets wrong

v0.5 is a `Batch[T]` that reads one `Source[T]`, groups by `Config`, and
runs each group through `Processor[T]`. The window math is the product.

| Problem | Consequence |
|---|---|
| One `T` for the whole pipeline | Multi-stage work falls back to `Batch[any]` |
| `Source.Read` returns two channels | Every source re-implements close and cancel |
| Errors are a side channel that must be drained | Undrained `errs` deadlocks; `IgnoreErrors` papers over the API |
| `*Item[T]` plus an engine ID | Allocation per item; the ID is not a correlation key |
| Each ready batch starts an unbounded goroutine | Fork bomb under load |
| Collector ignores context | Drain and abort are the same word |
| `Config` is an interface for four integers | `DynamicConfig` exists only because there is no `SetPolicy` |
| `Go` / `Done` / error channel | Three-step lifecycle where `Run` + `Close` would do |
| Request/reply is not expressible | N callers into one bulk call cannot ride a push stream |
| No fan-out / fan-in | Official answer is a small `flow/`, not a hub |
| Silent `fixConfig` | `MinItems: 0` becomes 1 with no error |
| `WithBufferConfig` panics after start | Misuse crashes the process |
| Documented Go 1.18, CI on 1.25/1.26 | Untested floor |
| Master is ahead of the latest tag | README does not compile under `go get` |

Worth keeping: min/max items, min/max wait, and their priority.

## 2. Three approaches

**A. Harden v0.5 and bolt on packages.** Smallest migration. Keeps
must-drain channels and one `T`. Not a redesign.

**B. Layered redesign (recommended).** `Batcher` + `Loader` + finite
`flow`. Delete `source` and `processor`. Matches `#71` and `#97`–`#100`.

**C. Dynamic keyed workflow.** Previous lineage. Expressible, not
adoptable on ShitQuant's capture or normalizer paths. `#98` forbids it
in v1. Encroaches on OnyxCore.

**Recommendation: B**, plus a Track 0 honesty tag of *current* master so
`go get` matches the README while B is built.

## 3. Goals and non-goals

Goals:

1. Stream batcher: `Run`, `Add`, `Close`; named drain and abort; a
   written scheduler; bounded items and handlers.
2. Request/reply: N `Do` calls, one handler, one terminal outcome per
   accepted `Do`. No implicit key coalescing.
3. Finite in-process flow: validated DAG, sequence, bounded parallel,
   fan-in, conditions, owned results, explicit join.
4. Shared Loaders across concurrent runs. The runner does not own them.
5. One observation *vocabulary*, duplicated types. No telemetry backend.

Non-goals: durable/distributed workflows; dynamic keyed tasks; promises;
watermarks; implicit coalescing; generic retries; priority across
loaders; byte-accurate memory limits; DSLs; UIs; depending on OnyxCore,
Redis, or a lock library; owning durability, time, or order.

## 4. Shape

```
github.com/MasterOfBinary/gobatch/batch    Batcher[T], Loader[In, Out], Policy
github.com/MasterOfBinary/gobatch/flow     Graph, Runner, ForEach
```

`flow` does not import `batch`. They compose in application code: a node
calls `Loader.Do`. Shared words (`ErrClosed`, `Event`, `Stats`) are
duplicated. `errors.Is(flow.ErrClosed, batch.ErrClosed)` is false.

Go floor **1.25** (oldest CI toolchain). `iter` for sources,
`testing/synctest` for wait tests. The 1.18 floor is dropped.

Options are `With…`. Invalid options fail at `New` / `NewLoader` /
`Compile` with an error. No public panic. `MinItems == 0` at construction means the default 1. `MinItems < 0`,
`MaxItems < 0`, and any duration `< 0` (`MinWait`, `MaxWait`,
`WithHandlerTimeout`, `WithShutdownBudget`) are `ErrInvalidPolicy`.
Zero on a duration means “none”, not a clamp of `-1`.

## 5. Package `batch`

### 5.1 Policy

```go
// A group is cut when any of these holds:
//   - MaxItems > 0 && len(forming) >= MaxItems
//   - MaxWait  > 0 && MaxWait elapsed since forming's first item
//   - len(forming) >= MinItems && MinWait elapsed since that first item
//   - flush or close is latched and forming is non-empty
//
// Cut is not dispatch. Dispatch is 5.2.
//
// Zero Policy is greedy: MinItems 1, no max, no waits. MaxItems 1 is
// one item per handler call.
//
// MaxItems == 0 means "no formation max". The item bound is still
// WithQueue (5.2). MaxWait == 0 means no linger.
// MaxItems > 0 && MaxItems < MinItems, or MaxWait > 0 && MinWait > MaxWait,
// is ErrInvalidPolicy. MinItems > queue is ErrInvalidPolicy.
type Policy struct {
	MinItems int
	MaxItems int
	MinWait  time.Duration
	MaxWait  time.Duration
}
```

Clocks start on the first item of the current forming group. An idle
batcher does not arm a timer. This is a hold-from-first-item, not a
debounce-from-last-item.

`SetPolicy` uses the same reject rules as `New` / `NewLoader`. After
`Close` or after `Run` has returned it returns `ErrClosed`. During `Run`
it is allowed: it stores the policy and wakes the `Run` loop. Only the
`Run` loop cuts. Shrinking `MaxItems` below `len(forming)` causes that
loop to cut groups of the new max in queue order before pulling the
next item. Wait clocks stay on the current forming group's first item.
If released slots are already `W`, further cuts wait for a slot — they
do not grow a third buffer. An observer must not call `SetPolicy` on
the same instance.

### 5.2 Scheduler (Batcher and Loader share this machine)

One inbound item queue of capacity `n` (`WithQueue`, default 1024,
`n < 1` is an error). One worker pool of size `W` (`WithWorkers`,
default 1, `n < 1` is an error). At most `W` handler calls. At most
`W` cut groups waiting for a worker. There is no third buffer.

```
              Add/Do
                 │
                 ▼
        inbound queue (≤ n)
                 │  Run loop pulls
                 ▼
            forming group
                 │  Policy cuts
                 ▼
        released slot (≤ W) ── worker free ──► handler (≤ W)
```

An item counts against `n` from the moment `Add`/`Do` accepts it until
its group is passed to a handler. A cut group that cannot start occupies
one of the `W` released slots; `Add`/`Do` feel that as a fuller inbound
queue. Formed-not-started work cannot grow without bound.

`Flush` and `Close` set flags the `Run` loop observes. They do not
hand a group to a worker. They may block only if a `WithObserver`
callback blocks; they do not wait for a worker.

`Run` is the only goroutine that cuts and dispatches, plus `W` worker
goroutines started before the first pull. No goroutine per queued item.

### 5.3 How a Batcher cuts vs how a Loader cuts

Same machine as 5.2. Cut is independent of a free worker. A cut group
parks in a released slot (≤ `W`) until a worker is free.

**Batcher.** When a Policy predicate holds, cut
`min(len(forming), MaxItems or len(forming))` in queue order. Zero
Policy: cut everything forming as soon as forming is non-empty (greedy
drain). A burst of four items is one group of four unless `MaxItems` is
smaller, even if four workers are idle.

**Loader.** When forming is non-empty:

- if a linger / `MaxItems` / flush / close predicate holds, cut one
  group of `min(len(forming), MaxItems or len(forming))`;
- else if `k` workers are free (not merely idle released slots), cut
  up to `k` groups of one (light load: one call per `Do`);
- else leave forming as-is so later arrivals coalesce.

That is the dataloader shape without a MinWait stall. `MaxItems` /
`MaxWait` still cut while all workers are busy; those groups occupy
released slots.

`NewLoader` and `SetPolicy` on a Loader reject `MinItems > 1` and
`MinWait != 0`.

### 5.4 Batcher

```go
type Handler[T any] func(ctx context.Context, items []T) error

func New[T any](h Handler[T], opts ...Option) (*Batcher[T], error)

func (b *Batcher[T]) Run(ctx context.Context) error
func (b *Batcher[T]) Add(ctx context.Context, item T) error
func (b *Batcher[T]) Flush()
func (b *Batcher[T]) Close()
func (b *Batcher[T]) SetPolicy(p Policy) error
func (b *Batcher[T]) Stats() Stats
func (b *Batcher[T]) Wait() // until Run has returned (or never started) and no handlers remain
func (b *Batcher[T]) Started() <-chan struct{} // closed once the loop accepts Add

func WithPolicy(p Policy) Option
func WithWorkers(n int) Option
func WithQueue(n int) Option
func WithObserver(f func(Event)) Option
func WithShutdownBudget(d time.Duration) Option // 0 = wait forever; default 0
```

`New(nil)` returns `ErrNilHandler` and cannot infer `T` without a typed
nil. `WithWorkers(n)` is not errgroup's `SetLimit(-1)`; `n < 1` is an
error.

**`Add`**

- After `Close` or after `Run` has returned: `ErrClosed`.
- Before `Run`: accepts into the inbound queue; if the queue is full,
  returns `ErrNotRunning` immediately (does not wait for `Run`).
- During `Run`, queue full: blocks until space, the waiter's
  `ctx.Err()`, `ErrClosed`, or abort (`Run`'s ctx). Abort returns
  `Run`'s `ctx.Err()` and does **not** accept the item. A successful
  `Add` (`nil`) means the item is accepted; abort will not later drop
  it silently. When several of those apply, the waiter's `ctx.Err()`
  wins, then abort, then `ErrClosed`.
- A handler must not call `Add` on the same Batcher in a way that needs
  a worker or `Run` to make progress. That returns `ErrReentry`.

**`Run`**

- Second call returns `ErrUsed` (not a panic), including a race on the
  first call: one wins, the other gets `ErrUsed`.
- Returns `nil` after a drain, `ctx.Err()` after an abort that finished
  in-flight work, `*GroupError` after a handler error, or
  `*ShutdownError` if the budget expired.
- Handler panic is **not** recovered. The worker goroutine crashes, as
  current `errgroup` does. Do not recover-and-re-raise from `Run`.
- The slice passed to a handler is the handler's. The engine will not
  reuse the backing array.

**`Consume`**

```go
func Consume[T any](ctx context.Context, b *Batcher[T], seq iter.Seq2[T, error]) error
```

Adds every item. Returns the first error from `seq` or `Add`. Does not
`Close` `b`. Cannot interrupt `seq` between yields.

### 5.5 Loader

A Loader is not “a Batcher with a reply slot.” Godoc's first sentence:
it does not coalesce by key. Equal `In` values are two operations.

```go
type Call[In, Out any] struct {
	In In
	// unexported identity, generation, reply
}

func (c *Call[In, Out]) Complete(out Out) bool
func (c *Call[In, Out]) Fail(err error) bool

type LoadHandler[In, Out any] func(ctx context.Context, calls []*Call[In, Out]) error

func NewLoader[In, Out any](h LoadHandler[In, Out], opts ...Option) (*Loader[In, Out], error)
func (l *Loader[In, Out]) Run(ctx context.Context) error
func (l *Loader[In, Out]) Do(ctx context.Context, in In) (Out, error)
func (l *Loader[In, Out]) Flush()
func (l *Loader[In, Out]) Close()
func (l *Loader[In, Out]) SetPolicy(p Policy) error
func (l *Loader[In, Out]) Stats() Stats
func (l *Loader[In, Out]) Wait() // until Run has returned (or never started) and no handlers remain
func (l *Loader[In, Out]) Started() <-chan struct{}

func WithHandlerTimeout(d time.Duration) Option // Loader only; 0 = none; default 0
// Default 0 is a deliberate exception to #71's "finite timeout": a
// default deadline would invent persist failure. Redeploy recipes set one.
```

`calls` is enqueue order, the same as a Batcher slice.

**Call states:** `queued | running | settled`. One `settle`. First
wins. `*Call` is never reused; a late `Complete` on a leftover pointer
cannot succeed a future `Do`. `Complete`/`Fail` must be the pointers in
the slice (a copy of the struct is a no-op that returns false).
`Fail(nil)` is `Fail(ErrUnanswered)`.

**`Do`**

- Before `Run`, or after `Run` has returned: `ErrNotRunning` /
  `ErrClosed`. Does not queue a waiter.
- After `Close`: `ErrClosed`. Drain does not accept new `Do`s. That
  is what makes “inbound and forming empty” a reachable condition.
- Queue full during `Run`: same wait rules as `Add` (space, waiter
  ctx, abort, `ErrClosed`).
- Waiter's `ctx` ending: `Do` returns `ctx.Err()`; the call stays in
  its group; a later settle is discarded. The caller does not know
  whether the handler acted. A caller that must know uses a context
  that does not end.
- One waiter's cancel does not cancel the handler context and does not
  fail sibling `Do`s.
- A LoadHandler must not call `Do` or `Run` on the same Loader
  (`ErrReentry` if it would block; always a mistake).

**Handler context** is derived from `Loader.Run`'s ctx, plus
`WithHandlerTimeout` if set. It is not any waiter's ctx.
`Loader.Run`'s ctx is process-scoped and must outlive every `Do`.
Cancelling it is process shutdown, not one enrichment ending.

**Handler error** fails unanswered calls in that group with that error
(or `ErrUnanswered` if the handler returned nil). Completed calls
stand. `Loader.Run` **continues**. A process-lifetime coalescer does
not die because one bulk call failed.

**Missing / extra / duplicate answers.** Unanswered at handler return
get the handler error or `ErrUnanswered` — never a zero `Out`. Extra
`Complete`/`Fail` after settle are ignored. The library does not
interpret a handler-built `map`; attribution is `Complete`/`Fail`.

### 5.6 Lifecycle table

Admission, `Close`, and `Run` ctx are the same mutex.

| Trigger | Unaccepted waiters | Accepted, handler not invoked | Handler invoked |
|---|---|---|---|
| `Close` (drain) | `ErrClosed` | formed under Policy until inbound and forming are empty; last incomplete group is released even if `< MinItems` | waited for (budget starts when nothing remains to dispatch) |
| `Run` ctx cancel (abort) | `Run`'s `ctx.Err()`; item not accepted | Batcher: dropped. Loader: settled `ctx.Err()` | handler ctx cancelled; budget starts now |
| `Close` ∪ cancel while work remains | abort wins | abort | abort |
| `Close` then `Wait` then cancel | n/a | n/a | already finished; cancel is a no-op |
| Batcher handler error | abort of the rest | dropped | waited; `Run` returns `*GroupError` |
| Loader handler error | none | none | unanswered in *that* group fail; Loader continues |
| Budget expiry | already `ErrClosed` / abort | already classified | handler ctx cancelled if not already; every still-unsettled `Do` is settled `*ShutdownError`; `Run` returns `*ShutdownError{Pending}`; late handler `Complete` is ignored; `Wait` joins the goroutines |

The budget clock starts once: when abort is latched, or when `Close`
has left nothing to dispatch and at least one handler is still running.
Default 0 waits forever. That is a deliberate exception to “`#95`
defaults to a finite grace”: a default timeout invents failure. Redeploy
recipes must set `WithShutdownBudget`. The *mechanism* is what `#71` /
`#95` require.

`Close` before `Run`: latches drain. A later `Run` drains what was
accepted and returns. `Add`/`Do` after that `Close` return `ErrClosed`.

`Wait` is always safe. It does not require a budgeted return.

Started means the handler function has been invoked.

A deadline is never evidence a write failed. A handler that must finish
a side effect already in flight uses `context.WithoutCancel`.

Two-context “stop reading, then drain” — do **not** use
`errgroup.WithContext` for this; the consume error would cancel `Run`:

```go
runCtx, stop := context.WithCancel(cleanupCtx)
defer stop()
b, err := batch.New(persist, batch.WithPolicy(batch.Policy{MaxItems: 500, MaxWait: 50 * time.Millisecond}))
done := make(chan error, 1)
go func() { done <- b.Run(runCtx) }()
consumeErr := batch.Consume(readCtx, b, records(readCtx))
b.Close()
runErr := <-done
```

`WithShutdownBudget(0)` (default) waits forever for in-flight handlers.
A non-zero budget is how `#95` / `#71` get a finite `Run`. After a
budgeted return, `Wait` is the only join. `ShutdownError.Pending` is
the handler count still running; there is no group list.

### 5.7 Errors and info

```go
type GroupError struct {
	Seq uint64
	Err error
}
func (e *GroupError) Error() string
func (e *GroupError) Unwrap() error

type ShutdownError struct {
	Pending int
	Err     error
}
func (e *ShutdownError) Error() string
func (e *ShutdownError) Unwrap() error

type Info struct{ Seq uint64 }
func InfoFromContext(ctx context.Context) (Info, bool) // handler ctx only

var (
	ErrClosed        = errors.New("batch: closed")
	ErrUsed          = errors.New("batch: Run already called")
	ErrNotRunning    = errors.New("batch: Run has not started")
	ErrUnanswered    = errors.New("batch: call not answered by handler")
	ErrReentry       = errors.New("batch: handler re-entered the same instance")
	ErrInvalidPolicy = errors.New("batch: invalid policy")
	ErrNilHandler    = errors.New("batch: nil handler")
)
```

Structured errors are pointers. `errors.As(err, &ge)` with
`var ge *GroupError`.

`WithHandlerTimeout` and `WithShutdownBudget` passed to `New` (Batcher)
return an error unless the Batcher also uses the budget (it does, so
budget is shared; timeout is Loader-only and rejected by `New`).

## 6. Package `flow`

### 6.1 Model

A **graph** is an immutable snapshot compiled from named nodes.
A **run** is one execution against one immutable input.
A **runner** admits a bounded number of concurrent runs and a bounded
number of executing *named* nodes.

A node function runs at most once per run. It receives the run input
and an immutable view of declared dependency outputs. It returns an
owned result. It does not mutate `in`. If `In` is a pointer, mutating
it is a user bug; the library does not copy.

`View.Get` returns the same `any` the producer returned. If that value
holds a slice, map, or pointer, two consumers share the heap. Mutating
it is a user bug. Deep-copy if you must detach.

There is no optional-edge engine, no dynamic `After`, no retry.

`ForEach` and `Do` inside a node body do **not** take executing-node
slots. The advertised bound is on named graph nodes only. A node that
starts 10k goroutines is a user bug. `#100`: a node blocked in `Do`
still occupies its node slot, so `WithMaxExecutingNodes` may be smaller
than a Loader's `MaxItems`; linger is what flushes those groups. That
is required behavior. Size the node bound for admission, the linger for
batch formation.

### 6.2 Building

```go
type View struct { /* immutable snapshot */ }
func (v View) Get(id string) (any, bool)

type Node[In any] func(ctx context.Context, in In, deps View) (any, error)
type Predicate[In any] func(ctx context.Context, in In, deps View) (bool, error)

func New[In any]() *Builder[In] // In must be spelled
func (b *Builder[In]) Node(id string, fn Node[In], deps ...string) *Builder[In]
func (b *Builder[In]) If(id string, pred Predicate[In], deps ...string) *Builder[In]
func (b *Builder[In]) Compile() (*Graph[In], error)
```

`Compile` snapshots. Later `Builder` mutation does not affect the
`*Graph`. The builder is not concurrency-safe.

`Compile` rejects: empty graph, empty ID, duplicate ID, unknown
dependency, self-dependency, cycle, more than 256 nodes, nil `fn` /
`pred`. It does not run.

```go
func (g *Graph[In]) Nodes() []string // compile order
```

**`If`**

- pred error → this node Failed; descendants Skipped (`ErrSkipDependency`)
- pred false → this node Success, `Condition=false`; descendants Skipped (`ErrSkipCondition`)
- pred true → this node Success, `Condition=true`; descendants may run

False is not a failure. No Else edge; write a second `If`.

**Dispatch.** One goroutine per run pulls a ready list. There is no
goroutine per waiting node.

A node is **runnable** iff every dep has `StatusSuccess` and no dep is
an `If` whose predicate was false.

- If the run is cancelled (caller ctx or runner shutdown) before it
  becomes runnable: `StatusCanceled`.
- If it is runnable and the run is cancelled before the function is
  invoked: `StatusCanceled`.
- If a dep Failed or was Skipped: `StatusSkipped` with
  `ErrSkipDependency` (or `ErrSkipCondition` when the blocking dep is
  a false `If`).
- Ready nodes wait on the run's ready list for an executing slot.
  They do not fail and they do not skip.

A node must not call `flow.Run` on the `Runner` executing it
(`ErrReentry`).

### 6.3 Running

```go
func NewRunner(opts ...RunnerOption) (*Runner, error)
func WithMaxConcurrentRuns(n int) RunnerOption // default 1; n < 1 error
func WithMaxExecutingNodes(n int) RunnerOption // default 8; n < 1 error
func WithMaxGraphNodes(n int) RunnerOption     // default 256; checked at Run
func WithObserver(f func(Event)) RunnerOption
func WithShutdownBudget(d time.Duration) RunnerOption // 0 = wait forever; default 0

func Run[In any](ctx context.Context, r *Runner, g *Graph[In], in In) (*Result, error)

func (r *Runner) Close()
func (r *Runner) Wait() // until no admitted run remains (always safe)

type Status int
const (
	StatusUnknown Status = iota
	StatusSuccess
	StatusFailed
	StatusSkipped
	StatusCanceled
)

type NodeOutcome struct {
	Status    Status
	Value     any   // owned result; nil if none
	Err       error // Failed, or skip/cancel reason
	Condition *bool // set on If nodes only
}

type Result struct {
	Status Status
	Nodes  map[string]NodeOutcome
}

func (res *Result) Outcome(id string) (NodeOutcome, bool)
```

`Run` is a package function. A generic method on `Runner` is illegal on
Go 1.25, and a `Runner[In]` would split the process-lifetime runner by
type.

`Run` dual-return:

- rejected (`ErrSaturated`, `ErrClosed` before admit, graph larger than
  the runner cap): `(nil, err)`
- admitted: `(*Result, err)` and `Result.Nodes` has every compiled id
- cancel after admit: `(res, ctx.Err())`
- node failure: `(res, *GraphError)`; independent branches still filled
- failure and cancel in one run: `Result.Status` is Failed;
  `err` is `*GraphError` (Canceled nodes are in `Result.Nodes` and in
  `GraphError.Nodes`)
- budget expiry: `(res, *ShutdownError)`; unfinished nodes are
  `StatusCanceled` in that `Result`. The `Result` is immutable after
  return. Late node returns are ignored.

Aggregate `Result.Status`: any Failed → Failed; else any Canceled →
Canceled; else Success. Skipped does not fail the run.

`Error()` on `*GraphError` uses `errors.Join` of `First` and every
value in `Nodes` so `errors.Is` walks them. There is no `Unwraps`.

A node that returns `ctx.Err()` while the run is not cancelled is
Failed, not Canceled. Ready-not-started nodes on cancel are Canceled.

Panic in a node or predicate is recovered to that node's Failed
(`*PanicError` wrapping `ErrPanic`). Capacity is released. The process
does not die.

`Close` stops admission atomically. If any runs are in flight it
starts the runner budget (default 0 = wait forever). After a non-zero
budget it cancels a child context each admitted `Run` derived for its
nodes (the caller's ctx is not cancelled). That is close-admission,
then grace, then cancel — `#98`. `Wait` joins. The runner does not
close Loaders.

A caller may still cancel `runCtx` first (abort). Redeploy recipes set
`WithShutdownBudget` on the runner and on each Loader.

```go
type GraphError struct {
	First error            // first Failed node, compile order
	Nodes map[string]error // Failed and Canceled only
}
func (e *GraphError) Error() string
func (e *GraphError) Unwrap() error // First; Error uses errors.Join

type PanicError struct {
	Node  string
	Value any
	Stack []byte
}
func (e *PanicError) Error() string
func (e *PanicError) Unwrap() error // ErrPanic

type ShutdownError struct {
	Pending int
	Err     error
}
func (e *ShutdownError) Error() string
func (e *ShutdownError) Unwrap() error

var (
	ErrClosed          = errors.New("flow: closed")
	ErrSaturated       = errors.New("flow: runner saturated")
	ErrReentry         = errors.New("flow: node re-entered the same runner")
	ErrPanic           = errors.New("flow: panic")
	ErrSkipCondition   = errors.New("flow: skipped (condition false)")
	ErrSkipDependency  = errors.New("flow: skipped (dependency)")
)
```

`flow.ShutdownError` is a distinct type from `batch.ShutdownError`.

### 6.4 ForEach (not a graph)

```go
// ForEach runs fn over items with at most limit in flight.
// First error cancels a child of ctx; it does not cancel the caller's
// ctx. Started calls are waited for. limit < 1 is an error. Empty
// items returns nil. items is snapshotted at the call.
func ForEach[T any](ctx context.Context, limit int, items []T, fn func(context.Context, T) error) error
```

`Sequence` and `Parallel` are not provided. Use `errgroup`. They would
disagree with Graph (independents finish) and with each other.

Unknown cardinality is a node body that calls `ForEach` after it has
the slice. There is no `Each` that returns handles.

### 6.5 Composition example

Process ctx outlives every run. Loaders are started before any `Do`.
Input is immutable. Results are owned. Persist has no linger until a
measurement asks; `MaxItems: 1` is a single `Do`.

```go
type Input struct{ Token Token }
type Proposal struct {
	Decoded Decoded
	Meta    Metadata
	Prices  []Price
}

procCtx, stop := context.WithCancel(context.Background())
// stop is registered first so it runs last: after Close+Wait, cancel is a no-op.
defer stop()

decodeL, _ := batch.NewLoader(decodeBulk, batch.WithPolicy(batch.Policy{MaxItems: 32, MaxWait: 20 * time.Millisecond}))
metaL, _ := batch.NewLoader(metaBulk, batch.WithPolicy(batch.Policy{MaxItems: 32, MaxWait: 20 * time.Millisecond}))
priceL, _ := batch.NewLoader(priceBulk, batch.WithPolicy(batch.Policy{MaxItems: 32, MaxWait: 20 * time.Millisecond}))
persistL, _ := batch.NewLoader(persistBulk, batch.WithPolicy(batch.Policy{MaxItems: 1}))

go decodeL.Run(procCtx)
go metaL.Run(procCtx)
go priceL.Run(procCtx)
go persistL.Run(procCtx)
<-decodeL.Started()
<-metaL.Started()
<-priceL.Started()
<-persistL.Started()

defer func() {
	decodeL.Close()
	metaL.Close()
	priceL.Close()
	persistL.Close()
	decodeL.Wait()
	metaL.Wait()
	priceL.Wait()
	persistL.Wait()
}()

g, err := flow.New[Input]().
	Node("decode", func(ctx context.Context, in Input, _ flow.View) (any, error) {
		return decodeL.Do(ctx, in.Token)
	}).
	Node("metadata", func(ctx context.Context, in Input, _ flow.View) (any, error) {
		return metaL.Do(ctx, in.Token)
	}).
	Node("prices", func(ctx context.Context, in Input, _ flow.View) (any, error) {
		return priceL.Do(ctx, in.Token)
	}).
	Node("evidence", func(ctx context.Context, _ Input, deps flow.View) (any, error) {
		d, _ := deps.Get("decode")
		m, _ := deps.Get("metadata")
		p, _ := deps.Get("prices")
		return check(Proposal{Decoded: d.(Decoded), Meta: m.(Metadata), Prices: p.([]Price)})
	}, "decode", "metadata", "prices").
	Node("persist", func(ctx context.Context, _ Input, deps flow.View) (any, error) {
		prop, _ := deps.Get("evidence")
		return persistL.Do(ctx, prop.(Proposal))
	}, "evidence").
	Compile()

runCtx, stopRun := context.WithCancel(context.Background())
defer stopRun()
runner, err := flow.NewRunner(flow.WithMaxConcurrentRuns(8), flow.WithMaxExecutingNodes(32))
res, err := flow.Run(runCtx, runner, g, Input{Token: tok})
```

Drain shutdown (this snippet): `Runner.Close`; `runner.Wait`; each
`Loader.Close`; each `Loader.Wait`; then the defers' `stop`/`stopRun`
are no-ops. Abort shutdown: `stopRun()` / `stop()` first, then Close
and Wait. Do not Close and cancel while work remains if you meant drain.

Type assertions at the join are the cost of a registry-free graph. The
join is ordinary Go; that is the explicit join `#98` asked for.

## 7. Observation

Each package has its own `Event`. A `func(batch.Event)` is not a
`func(flow.Event)`.

```go
type EventType int
const (
	EventAdmitted EventType = iota + 1
	EventReleased
	EventStarted
	EventCompleted
	EventFailed
	EventSkipped
	EventCanceled
)

type Cause int
const (
	CauseMaxItems Cause = iota + 1
	CauseMaxWait
	CauseMinWait
	CauseFlush
	CauseClose
	CauseCondition
	CauseDependency
)

type Event struct {
	Seq  uint64
	Name string
	Type EventType
	Size int
	Err  error
	Cause Cause
}

type Stats struct {
	Queued   int
	InFlight int
	Released uint64
	Items    uint64
	Failed   uint64
}
```

Callback runs outside internal locks, on the transition goroutine. No
internal queue. Panic recovered. No payloads. Must not call back into
the same instance (`ErrReentry` or deadlock). `Flush`/`Close` only
signal; the observer for a cut runs on the `Run` goroutine so `Flush`
does not wait for workers.

`Stats` is a snapshot under one mutex hold.

## 8. Consumer mapping

### 8.1 ShitQuant (path-level)

House rules used here: batching only after a measurement; never
manufacture time or order; a deadline is never evidence a write failed;
cancellation is not a drain; do not put a keyed scheduler on capture;
do not replace the normalizer's pending maps; do not recreate the
archived hub.

| Path | Use gobatch? |
|---|---|
| Capture journal | No. Abort drops queued-not-started items. |
| Helius-style RPC scheduler | No. Per-kind retry and forever dedup stay in the app. |
| Normalizer pending maps | No. Completing-record order is app-owned. |
| Recorder / group commit | No. Durable-before-visible is an app transactional API. |
| Enrichment / proposal | Yes. `flow` vs a direct-call/replay baseline (`#97`–`#99`). |
| Multi-token sampling | Yes. Shared `Loader`s (`#71`, `#100`). |
| Phase-5 flush | Yes, after a measurement. `Batcher` + `MaxWait`. |
| Paper / replay | Only if the graph *is* the enrichment path. Clock and order stay in the app. |

Adoption order: pin a release, Loader+flow on enrichment, measure the
flush, never on capture.

File-level rates, line counts, and lock names from the prior private-tree
review are not restated here. They informed the Yes/No table; they are
not claims this environment re-verified.

### 8.2 OnyxCore (medium)

`#97`: build `flow/` in GoBatch; do not delete or archive OnyxCore; do
not depend on it. Public 2023 forks of imgui node editors suggest a
richer host, possibly visual.

Leave room: stable node IDs, explicit outcomes, observation events.
Do not absorb: dynamic graphs, named ports, plugins, ImGui, cron, a
job database, workers. Those are why OnyxCore still exists.

`cutgraph` is a public Python video-timeline repo. It adds no
requirements.

### 8.3 Shitlock (low)

No public repo. A lock library — distributed, position, or
single-writer — should not live in gobatch. If one appears, it is a
dependency of the application or of a future plane, not a package here.

### 8.4 Matrix

| Capability | ShitQuant | OnyxCore | Shitlock |
|---|---|---|---|
| Stream Batcher | Useful after measurement; harmful on capture | Weakly useful | Unused |
| Loader | Required off capture | Useful as a callee | Unused |
| Static flow | Required for enrichment | Useful as a callee; must not replace OnyxCore | Unused |
| Dynamic keyed workflow | Harmful | OnyxCore-owned if it exists | Unknown |
| Generic retry | Harmful | Out of `flow/` | Lock-library owned |
| Durable jobs | App-owned | OnyxCore-owned if any | Unused |
| Locks | App-owned | Unused | Would provide them |
| Observation | Useful | Useful as hooks, not a UI | Unused |
| Bounds | Required | Useful if it embeds | Own wait bound |

## 9. Migration from v0.5

| v0.5 | Redesign |
|---|---|
| `New` + `Go` + `Done` | `New(handler, WithPolicy(p))` + `Run` + `Close` |
| `Source.Read` | `Consume` or a loop around `Add` |
| `Processor.Process` | `Handler[T]` |
| `Item[T]` | `T` |
| error channel | `Run`'s return |
| `Config` / `DynamicConfig` | `Policy` / `SetPolicy` |
| `BufferConfig` | `WithQueue` |
| `RunBatchAndWait`, `ExecuteBatches` | `errgroup` |
| `processor.*`, `source.*` | a loop |
| (roadmap) sync-like batching | `Loader` |
| (none) | `flow` |

No shims. The Track 0 tag of current master is the pin for apps that
are not moving.

## 10. Production roadmap

### Track 0 — honesty (current API)

Fill `[Unreleased]` for `#64` and `#66`. Tag so `go get` compiles the
README (v0.5.1 vs v0.6.0: maintainer call). Raise `go.mod` to 1.25 or
add a 1.18 CI leg. Reconcile stale PRs `#65`, `#67`, `#68`, `#76`.
Verify an external module with `GOWORK=off`.

### Track 1 — `batch` (v0.7.0)

`Policy`, scheduler, `Batcher`, `Consume`, `Loader`, errors, observer.
Delete `processor` and `source`. Rewrite docs.

Tests under `synctest`: policy table; greedy Batcher vs light-load
Loader; cut while workers busy (released ≤ W); invalid policy;
`MinItems == 0` default vs negative reject; negative durations
rejected; `Add` abort does not return nil then drop; `Do` after
`Close` is `ErrClosed`; `Started` before first `Do`;
drain vs abort with the two-context sample; `Add`/`Do` before `Run`;
`Add` after `Run` returned; full-queue backpressure; formed-not-started
≤ `W`; worker bound; `SetPolicy` cut and Loader reject; unanswered
calls; `Do` with ended ctx; equal `In` is two ops; caller cancel does
not cancel siblings; budget settles Loader waiters; late `Complete` on
a leftover pointer; `ErrReentry`; `Run` twice is `ErrUsed`; handler
slice not reused; race detector.

Waits must be durable (chan / timer / cond), not mutex-only, or
`synctest.Wait` hangs. Observer tests use a buffered chan and do not
`Wait` from the callback. Timeouts in tests are explicit, not defaults.

### Track 2 — `flow` (v0.8.0)

`Builder`, `Compile` snapshot, `Runner`, `Run`, `If`, `ForEach`.
Examples: enrichment fixture (no paid APIs) and a non-trading graph.

Tests: diamond; cycle/unknown/duplicate; condition false vs error;
skipped descendants; independent branch completion; `ErrSaturated`;
cancel at each state; panic recovery; concurrent runs isolated;
`Compile` then mutate builder; `If` false does not run descendants;
owned results visible only via `View`; `#100` linger when node bound
< `MaxItems` as a follow-on; admission/close races.

`#100` does not block `#99`.

### Track 3 — v1.0

Freeze after Track 0 exists, Track 1–2 have a second example, and an
external pin with `GOWORK=off` compiled against the *new* surface.

### Track 4 — not this redesign

Redis (`#72`). Optional `WithCoalesce`. Promise / Watermark / keyed
workflow (OnyxCore or a new proposal with a measurement). Visual
editor. `WithCost`. `DoAll`.

### Checklist

| Item | When |
|---|---|
| Tag matches docs | Track 0 |
| Go floor tested | Track 0 / 1 |
| Written scheduler, formed ≤ W | Track 1 |
| Named drain vs abort, cancel wins | Track 1 |
| One Call state machine | Track 1 |
| No public panic | Track 1 |
| Owned results + explicit join | Track 2 |
| Bounded runs and named nodes | Track 2 |
| Shared Loader + flow example | Track 2 follow-on |
| External pin | Track 0 and v1 |

## 11. Open questions

1. Honesty tag number (v0.5.1 vs v0.6.0 of the current surface).
2. Whether a later `WithCoalesce(func(In) K)` is worth it. Out of v1.
3. `View.Get` type assertions vs generated join helpers. v1 is `Get`.
4. Settled: `flow.Run` is a package function; handler timeout default 0;
   `Sequence`/`Parallel` are not shipped.

## 12. Difference from the previous redesign branch

| Previous `workflow` lineage | This proposal |
|---|---|
| Keyed tasks, Promise, Watermark, Sequence, Remember, retry | Finite DAG, owned results, no retry |
| Shared mutable kinds | Immutable input + `View` |
| Recommended replacing Helius / joining in the normalizer | Forbids both |
| `Each` / `After` / handles | `ForEach` + declared deps |
| Priority `Pool` | Out |
| One `workflow` package | `batch` then `flow` |
| `Run`/`Add`/`Close`, Loader, Policy, Go 1.25 | Kept, then specified as a scheduler |
