# GoBatch redesign proposal: batching and workflows

Status: revision 2, after three independent reviews (adversarial, consumer
fit, Go API design). The reviews and what changed because of them are in
[REVIEWS.md](REVIEWS.md). This document proposes a from-scratch redesign of
GoBatch (v0.6, breaking): a batching primitive, request/reply batching, and a
workflow scheduler for keyed tasks with dependencies, readiness, fan-out and
fan-in. It is written against one concrete consumer, ShitQuant, and section 6
says honestly where it fits that consumer today and where it does not.

## 1. What the current library gets wrong

The v0.5 library is a single `Batch[T]` that reads one `Source[T]`, groups items
by a `Config`, and runs each group through a chain of `Processor[T]`. It works,
but several structural choices limit it:

| Problem | Consequence |
|---|---|
| One type parameter for the whole pipeline | A stage cannot change the item type; multi-type pipelines fall back to `Batch[any]` and assertions |
| `Source.Read` returns two channels | Every source re-implements goroutine, close and cancellation discipline; a nil channel is a runtime error |
| Errors are a side channel that must be drained | An undrained channel deadlocks the pipeline once its buffer fills; helpers exist only to work around this |
| Per-item `*Item[T]` with an engine-assigned ID | One allocation per item; the ID means nothing to the user and is unusable as a dedup or ordering key |
| Batches run in unbounded goroutines | No concurrency limit, no ordering, no backpressure beyond channel buffers |
| Context cancellation does not stop processing | "Drain" and "abort" are conflated; there is no abort and no graceful close other than closing the source |
| `Config` is an interface for four integers | `DynamicConfig` exists only because there is no `SetPolicy` |
| Single-use `Batch` with `Go`/`Done`/error channel | Three-step lifecycle where `Run(ctx)` plus `Close()` would do |
| Request/reply batching (the roadmap's "sync-like batching") is not expressible | The most common real batching need, coalescing N callers into one call, cannot be built on top |
| No notion of dependency, readiness, join, fan-out or fan-in | Consumers hand-write schedulers and pending-state machines |

The one thing worth keeping is the batching policy: min/max items, min/max
wait, and their priority. Everything else is replaced.

## 2. Goals and non-goals

Goals:

1. A batching primitive that is hard to misuse: `Run(ctx)`, `Add`, `Close`;
   bounded workers; bounded queue; drain and abort as two distinct words.
2. Request/reply batching: N concurrent callers, one handler call, each caller
   gets its own reply.
3. A workflow scheduler for keyed tasks with dependencies, externally resolved
   promises, monotone watermarks, fan-out, fan-in, retry, and a release order
   that is a deterministic function of the consumer's calls when the consumer
   needs that.
4. Small: two packages, functions over interfaces, no goroutine the user did
   not ask for, and every rule statable in one godoc sentence.
5. Never manufacture time or order; never hide at-least-once delivery; never
   own durability. Those belong to the consumer.

Non-goals:

- Durable or distributed workflows. The scheduler is in-process and forgets
  finished work; idempotency across restarts is the consumer's journal.
- A DSL or config-file graph. Graphs are Go code.
- Stream semantics (event-time windows, exactly-once). The library has no
  clock other than wall-clock waits the consumer configures.
- Shared capacity with priorities across kinds. A strict priority scheduler
  starves; an aging one is policy. A consumer wraps its client in a semaphore.

## 3. Shape

```
github.com/MasterOfBinary/gobatch/batch      Batcher[T], Loader[In, Out], Policy
github.com/MasterOfBinary/gobatch/workflow   Workflow, Kind, Promise, Watermark, Handle
```

`workflow` depends on `batch`; `batch` depends only on the standard library.
Go floor 1.25: `iter` for sources and `testing/synctest` for every timer test,
so no clock seam appears in the public API. `processor` and `source` are
deleted; a source is an iterator, a processor is a function.

Both packages use functional options named `With…`. There are no other
option families; per-kind settings are a config struct.

## 4. Package `batch`

### 4.1 Policy

```go
// Policy decides when the pending group is released to a handler.
//
// A group is released as soon as any of these holds:
//   - len(group) >= MaxItems                                      (MaxItems > 0)
//   - MaxWait has elapsed since the group's first item was queued (MaxWait > 0)
//   - len(group) >= MinItems && MinWait has elapsed since the first item
//   - Flush or Close was called
//
// MinItems below 1 is 1; a zero MaxItems or MaxWait means no maximum; a max
// smaller than its min lowers the min. The zero Policy is greedy: a group is
// released as soon as one item is queued and a worker is free, and it takes
// everything queued at that moment. MaxItems 1 therefore means one item per
// handler call.
type Policy struct {
	MinItems int
	MaxItems int
	MinWait  time.Duration
	MaxWait  time.Duration
}
```

`MaxWait` is measured from the first item of the group, which differs from
v0.5's timer-from-collection-start; the difference is that an idle batcher
never fires an empty timer.

### 4.2 Batcher

```go
// Handler processes one released group. The slice is the handler's after the
// call. Returning an error stops the batcher: Run returns a *GroupError and
// in-flight handlers see a cancelled context. A handler that wants to
// continue after a failure handles it and returns nil.
type Handler[T any] func(ctx context.Context, items []T) error

type Batcher[T any] struct{ /* unexported */ }

func New[T any](h Handler[T], opts ...Option) *Batcher[T]

// Run executes the batcher until Close drains it or ctx ends. It returns nil
// after a drain, ctx.Err() after an abort, or the *GroupError that stopped
// it. Run panics if called twice. On any return, every in-flight handler has
// returned, and Add returns ErrClosed from then on.
func (b *Batcher[T]) Run(ctx context.Context) error

// Add queues one item. It blocks while the queue is full and returns
// ctx.Err() or ErrClosed instead of queuing; when both apply, ctx.Err() wins.
// Add before Run queues up to the queue size and then blocks until Run starts.
func (b *Batcher[T]) Add(ctx context.Context, item T) error

// Flush releases the pending group now. It never blocks.
func (b *Batcher[T]) Flush()

// Close stops accepting items; Run drains what was accepted, waits for
// in-flight handlers and returns nil. Close is idempotent and safe before Run.
// If ctx ends after Close, abort wins over drain.
func (b *Batcher[T]) Close()

// SetPolicy replaces the policy; the pending group is re-evaluated under it.
func (b *Batcher[T]) SetPolicy(p Policy)
func (b *Batcher[T]) Stats() Stats // Queued, InFlight, Groups, Items; a snapshot

func WithPolicy(p Policy) Option
func WithWorkers(n int) Option // concurrent handler calls; default 1
func WithQueue(n int) Option   // items Add may queue before blocking; default 1024
```

Groups are handed to workers in release order; that is the only ordering
promise, and with `WithWorkers(1)` it is total.

```go
// GroupError is what Run returns when a handler failed.
type GroupError struct {
	Seq     uint64 // release sequence of the group, from 1
	Attempt int    // 1 unless a workflow retried the group
	Err     error
}
func (e *GroupError) Unwrap() error

// Group describes the group a handler is serving.
type Group struct{ Seq uint64; Attempt int }
func GroupFromContext(ctx context.Context) (Group, bool)

var ErrClosed = errors.New("batch: closed")
```

Lifecycle, two words with one meaning each:

- **Close** means drain: everything accepted is processed.
- **Context cancellation** means abort: queued items are dropped, blocked
  `Add` calls return, in-flight handlers see a cancelled context and are
  waited for. A handler that must finish a side effect (journal a reply that
  already arrived) does so under `context.WithoutCancel`. A deadline is never
  evidence that a write failed.

A consumer that wants "stop reading at the deadline, then drain what was
read" uses two contexts. The first review caught this example with one:

```go
g, runCtx := errgroup.WithContext(cleanupCtx)          // cleanup deadline: abort
b := batch.New(persist, batch.WithPolicy(batch.Policy{MaxItems: 500, MaxWait: 50 * time.Millisecond}))
g.Go(func() error { return b.Run(runCtx) })
g.Go(func() error {
	defer b.Close()                                       // reader done: drain
	return batch.Consume(readCtx, b, reader.Records(readCtx)) // read deadline: stop reading
})
return g.Wait()
```

Panics in handlers are recovered in the worker and re-raised from `Run`, as
`errgroup` does, so the stack is the handler's and the process still dies.

### 4.3 Sources

```go
// Consume adds every item of seq to b and returns the first error from seq
// or Add. It cannot interrupt seq between yields, only the Add. It does not
// Close b.
func Consume[T any](ctx context.Context, b *Batcher[T], seq iter.Seq2[T, error]) error
```

`iter.Seq2[T, error]` is the whole source contract. A channel is
`for v := range ch { b.Add(ctx, v) }`; no adapter is exported.

### 4.4 Loader: request/reply batching

```go
// Call is one caller's request awaiting a reply. Complete and Fail answer
// it; the first answer wins and later ones are ignored and counted.
type Call[In, Out any] struct{ In In /* unexported reply slot */ }
func (c *Call[In, Out]) Complete(out Out)
func (c *Call[In, Out]) Fail(err error)

// LoadHandler serves one group of calls. Calls unanswered when it returns are
// failed with the returned error, or with ErrUnanswered if it returned nil.
type LoadHandler[In, Out any] func(ctx context.Context, calls []*Call[In, Out]) error

// Loader coalesces concurrent Do calls into LoadHandler calls (dataloader
// shape). It is a Batcher[*Call[In, Out]] with a reply per item.
type Loader[In, Out any] struct{ /* unexported */ }
func NewLoader[In, Out any](h LoadHandler[In, Out], opts ...Option) *Loader[In, Out]
func (l *Loader[In, Out]) Run(ctx context.Context) error
func (l *Loader[In, Out]) Close()
func (l *Loader[In, Out]) Flush()
func (l *Loader[In, Out]) SetPolicy(p Policy)
func (l *Loader[In, Out]) Stats() Stats

// Do queues in and waits for its reply. If ctx ends first, Do returns
// ctx.Err(); the call stays in its group and its reply is discarded, so the
// caller does not know whether the handler acted on it. A caller that must
// know uses a context that does not end.
func (l *Loader[In, Out]) Do(ctx context.Context, in In) (Out, error)
```

This is the roadmap's "sync-like batching". The zero policy is what a
dataloader does: while a worker is busy, everything that arrives coalesces
into the next call.

## 5. Package `workflow`

### 5.1 Model

A **workflow** owns **kinds**, **promises** and **watermarks**.

- A **kind** is a typed unit of work `In -> Out` executed by a handler over
  groups of ready tasks, with its own `batch.Policy`, worker count, retry
  policy and memory. One `batch.Batcher` per kind.
- A **task** is one instance of a kind, identified by `(kind, key)`, with a
  set of dependencies.
- A **promise** is a keyed, complete-once value the consumer resolves from
  outside. It is what a task depends on when the thing it waits for is not
  work but an arrival: "the confirmed block for slot S, whenever its record
  is read, or never".
- A **watermark** is a monotone counter tasks can wait on: "the chain root
  reached slot S".
- A **handle** is a future for any of the above.

A task is **ready** when every dependency is complete. Ready tasks go to
their kind's batcher in **readiness order**: the order in which the event
that completed their last dependency happened, and, within one event that
readies several tasks, registration order. Completion satisfies dependents.
That is the whole scheduler.

Fan-out is one handle with several dependents. Fan-in is one task with
several dependencies. A pipeline is the case where every task has one.

```
             ┌─────────────┐
             │ Workflow    │  task table, dependency edges, promises,
             │  scheduler  │  watermarks, readiness order, retry
             └──────┬──────┘
                    │  ready tasks -> the kind's batch.Batcher
       ┌────────────┼────────────┐
       ▼            ▼            ▼
   Decode        Metadata      Prices        three kinds (or promises, when the
       │            │            │           data arrives instead of being computed)
       └────────────┼────────────┘
                    ▼
                 Persist                     a kind whose tasks depend on all three
```

### 5.2 Defining kinds

```go
type Workflow struct{ /* unexported */ }
func New(opts ...Option) *Workflow
func WithObserver(f func(Event)) Option

// Run runs the scheduler and every kind's batcher until Close drains
// everything or ctx ends. Panics if called twice.
func (w *Workflow) Run(ctx context.Context) error
// Close stops root submissions. Idempotent, non-blocking, safe before Run.
func (w *Workflow) Close()
func (w *Workflow) Stats() Stats // per kind: Pending, Ready, InFlight, Done, Failed, Retried

type KindConfig struct {
	Policy   batch.Policy
	Workers  int          // 0 means 1
	Retry    *RetryPolicy // nil: no retry
	Remember bool         // keep finished keys deduplicated for the run (5.4)
	Sequence *Sequence    // release in explicit ordinal order instead of readiness order (5.6)
}

type Kind[In, Out any] struct{ /* unexported */ }
func Define[In, Out any](w *Workflow, name string, h Handler[In, Out], cfg KindConfig) *Kind[In, Out]

// Handler serves one group of ready tasks. Tasks unfinished when it returns
// are failed with the returned error, or ErrUnanswered if it returned nil.
type Handler[In, Out any] func(ctx context.Context, tasks []*Task[In, Out]) error

type Task[In, Out any] struct {
	Key     Key
	In      In
	Attempt int
}
func (t *Task[In, Out]) Complete(out Out) // first answer wins; later answers are ignored and counted
func (t *Task[In, Out]) Fail(err error)
```

The workflow's `Handler` is not `batch.Handler`; the doc for each says so.

### 5.3 Submitting and depending

```go
type Key string

// Handle is a future. The zero Handle is never done. A Handle is a Dep.
type Handle[Out any] struct{ /* unexported */ }
func (h Handle[Out]) Result(ctx context.Context) (Out, error) // never call inside a handler (5.8)
func (h Handle[Out]) Done() <-chan struct{}

// Submit registers a root task. It blocks while the workflow has MaxOpen
// roots whose descendants are not all finished (5.5), and returns ctx.Err()
// or ErrClosed instead. Never call it inside a handler.
func (k *Kind[In, Out]) Submit(ctx context.Context, key Key, in In) (Handle[Out], error)

// After registers a task that depends on deps. build runs in this kind's
// worker once every dep is complete, immediately before the handler; it may
// call Result on the deps, which return at once, and an error from it fails
// the task. After never blocks and may be called from inside a handler.
func (k *Kind[In, Out]) After(key Key, deps []Dep, build func(ctx context.Context) (In, error)) Handle[Out]
```

Every registration dedups by `(kind, key)`: a second registration while the
first is unfinished returns the first's handle and keeps the first `in`. With
`Remember`, finished keys dedup too, for the life of the run. Without it, a
finished key resubmitted runs again; the window between finishing and
resubmitting is inherent, so a consumer that needs "once per key per run"
sets `Remember`.

A dependency failing fails its dependents with `*DependencyError` without
running them. Handles own results: a handle can gain dependents after
completion, and they are ready at once. The scheduler holds no result of its
own once a task is finished.

Two entry points only. `Then`, `Join2`, `Join3`, `JoinAll` and `Each` from
revision 1 are gone: `After` with `Result` on satisfied deps covers every
fan-in, and fan-out of unknown cardinality is a handler calling `After` per
element, which is non-blocking and so is safe there.

Verified with the toolchain: `Define`, `Submit` and `After` infer every type
parameter; making `Submit` and `After` methods on `*Kind` is what lets an
interface-typed `In` accept a concrete argument.

### 5.4 Promises and watermarks

```go
// Promise is a keyed, complete-once value resolved from outside the workflow.
type Promise[Out any] struct{ /* unexported */ }
func NewPromise[Out any](w *Workflow, name string) *Promise[Out]

// Resolve completes key with out. The first Resolve wins; later ones are
// ignored and counted, and Resolve returns false for them. It never blocks
// and may be called from any goroutine, including a handler.
func (p *Promise[Out]) Resolve(key Key, out Out) bool
func (p *Promise[Out]) Reject(key Key, err error) bool

// Handle returns the handle for key, registering it if unseen.
func (p *Promise[Out]) Handle(key Key) Handle[Out]

// Watermark is a monotone counter with waiters. Usable without a workflow.
type Watermark struct{ /* unexported: value, generation channel swapped on Advance */ }
func NewWatermark() *Watermark
func (m *Watermark) Advance(v uint64)                   // never decreases
func (m *Watermark) Value() uint64
func (m *Watermark) Wait(ctx context.Context, v uint64) error // Value() >= v, or ctx.Err()
func (m *Watermark) At(v uint64) Dep
```

A promise is what a "pending map" is: swaps waiting for their block are tasks
depending on `blocks.Handle(slotKey)`; a block arriving before any swap is
`Resolve` on a key nobody has asked for yet, and a later `After` on it is
ready at registration. A promise never resolved by `Close` is reported, not
guessed (5.9).

A watermark is a readiness that is "the world advanced to v". Its only
operation is `Advance`, so the library never invents progress. `Wait` is the
generation-swap broadcast a journal follower hand-writes; it does not model
"closed", which a follower still expresses with its own sentinel.

### 5.5 Bounds

```go
func WithMaxOpen(n int) Option // roots whose descendants are not all finished; Submit blocks past it
```

Continuations (`After`, `Resolve`, `Advance`) never block: a blocked handler
holding a worker slot while waiting for a queue that handler feeds is a
deadlock, and revision 1 had that deadlock in the scheduler itself. So the
only backpressure point is root `Submit`, and for it to bound anything, a
root must count until its whole subtree is finished. Each task carries its
root; a per-root counter of unfinished descendants is what `MaxOpen` reads.
Memory is bounded by `MaxOpen` times the largest subtree a root produces;
the library states the bound and does not cap fan-out.

Ready queues are unbounded (bounded by `MaxOpen`), so the scheduler never
blocks on a batcher: batchers pull ready tasks from the scheduler.

### 5.6 Release order

Default: readiness order (5.1). It is deterministic when the events that
complete dependencies are deterministic: `Resolve`, `Advance` and `Submit`
issued from one goroutine in a fixed order, and kinds with `Workers: 1`
whose own inputs were deterministic. It is not deterministic across kinds
with several workers, and the library says so rather than pretending a
reorder buffer can fix it.

Explicit: `Sequence`.

```go
// Sequence orders a kind's releases by consumer-assigned ordinals.
type Sequence struct{ /* unexported */ }
func NewSequence() *Sequence
// Settle declares that every task with ordinal <= n has been registered.
func (s *Sequence) Settle(n uint64)

// Ordinal sets the ordinal of the task registered by the next Submit/After
// on a sequenced kind; registration without one panics.
func (k *Kind[In, Out]) Ordinal(n uint64) *Kind[In, Out] // returns a view; the kind itself is unchanged
```

A sequenced kind releases a task with ordinal `n` once it is ready, every
registered task with a smaller ordinal has been released, and `Settle(m)`
has been called for some `m >= n`, so nothing smaller can still appear. The
consumer knows when a record's tasks are all registered; the library does
not, so the consumer says. The cost is head-of-line blocking: a task whose
dependency never arrives blocks everything behind it, so a sequenced kind
must depend only on things that always complete (a computation, not an
arrival). Revision 1's implicit ordinals inherited from a first dependency
could not be made to satisfy this and are gone.

### 5.7 Retry

```go
type RetryPolicy struct {
	Attempts int                  // total, including the first
	Budget   time.Duration        // measured from the task's readiness instant; 0 means none
	Backoff  func(attempt int) time.Duration
	Retry    func(err error) bool // nil: everything
}
```

One rule, on the handler: a handler error fails every unfinished task in the
group with that error. If the kind has a policy and `Retry(err)` is true,
the unfinished remainder is re-invoked as one group after `Backoff`, with the
same `batch.Group.Seq` and `Attempt+1`, until `Attempts` or `Budget` is
exhausted; then those tasks fail with `*RetryError`. Tasks the handler
already completed are not re-sent. A task failed with `Fail(err)` is retried
alone under the same policy. A retrying group holds no worker while it waits.

A task failure is data: it fails handles and dependents, and `Run` goes on.
The only way a handler ends the run is returning `workflow.Fatal(err)`, on
which `Run` aborts and returns `err`. Failures that must latch (a persist
error) are `Fatal`; failures that are outcomes (an RPC budget exhausted) are
not.

### 5.8 Rules for handlers

Stated once, in the `Handler` doc, and checked in the race tests:

- Never call `Submit`, `Result` or `Watermark.Wait` inside a handler; each
  can block on the scheduler the handler is part of.
- `After`, `Resolve`, `Advance`, `Complete`, `Fail` are always safe.
- Answer every task before returning; a goroutine answering after return
  races the auto-fail and loses.

### 5.9 Lifecycle

- `Run` returns when `Close` has been called and nothing is ready, in flight
  or backing off; or when ctx ends. Before returning it fails every task
  still pending on a promise, watermark or sequence with `*IncompleteError`,
  which lists them as `{Kind, Key, Waiting Dep}`, so every handle resolves.
  If tasks were pending, `Run` returns that error.
- After `Run` returns, `Submit` returns `ErrClosed` and `Resolve`, `Advance`,
  `After` are no-ops that return zero handles already failed.
- Abort fails every pending task with ctx.Err() and waits for in-flight
  handlers.
- Panics propagate as in 4.2.

Task state machine, with the goroutine that drives each transition:

| From | To | Driven by |
|---|---|---|
| registered | ready | scheduler, when the last dep completes (`Resolve`/`Advance`/`Complete` caller's goroutine, under the scheduler lock) |
| ready | building | the kind's worker |
| building | in handler | the kind's worker (`build` ok) |
| building | failed | the kind's worker (`build` error) |
| in handler | done | `Complete` (handler goroutine) |
| in handler | failed | `Fail`, handler error, `ErrUnanswered` (handler goroutine) |
| failed (retryable) | backing off | scheduler timer |
| backing off | ready | scheduler timer |
| registered/ready | failed | `Run` return (`*IncompleteError` or ctx.Err()), dependency failure |

### 5.10 Observation

```go
type Event struct {
	Kind    string
	Key     Key
	Type    EventType // Registered, Ready, Started, Completed, Failed, Retried
	Attempt int
	Err     error
}
```

`WithObserver(func(Event))` is called synchronously on the goroutine that
made the transition, so an observer must be cheap. `Stats` is a snapshot.

### 5.11 Errors

```go
type DependencyError struct{ Kind string; Key Key; Err error } // Unwrap
type TaskError       struct{ Kind string; Key Key; Err error } // what Result returns for a failed task; Unwrap
type RetryError      struct{ Attempts int; Err error }         // Unwrap
type IncompleteError struct{ Pending []PendingTask }
type PendingTask     struct{ Kind string; Key Key; Waiting Dep }
var ErrClosed     = batch.ErrClosed
var ErrUnanswered = errors.New("workflow: task not answered by handler")
func Fatal(err error) error
```

## 6. ShitQuant, honestly

The consumer-fit review found revision 1's section 6 wrong in six of seven
claims. The corrected picture:

- The normalizer never fetches: it reads capture records and is network-free
  by invariant. Its pending map is a keyed, externally completed promise.
- It admits in **completing-record** order, and replay availability is the
  completing record's receipt time. Ordering by first dependency, as
  revision 1 did, would reorder admissions and, through the non-decreasing
  availability clamp, change the meaning of `available_time`.
- It is already deterministic by construction, because one goroutine reads
  records in order.
- The Helius scheduler lives on the capture path, whose bytes cannot be
  rebuilt; that is the largest blast radius in the repo, not the smallest.
- The house rule is "batching only after a measurement asks for it". The
  only measurement on file is the capture journal's (3.42 records/s, 10.45 ms
  mean commit) and it says keep per-record commits; the admission log's
  measurement was written and never run.

So the honest mapping is a validation of the library's expressiveness, not
an adoption plan. Adoption order, if any: 6.4 first; 6.2 is expressible but
not adoptable in part; 6.1 never.

### 6.1 The block-metadata scheduler (`internal/helius/scheduler.go`)

Expressible, with the corrections the review demanded:

```go
w := workflow.New(workflow.WithObserver(countUnresolved))
root := workflow.NewWatermark()
confirmed := workflow.Define(w, "block.confirmed", fetchConfirmedBlocks,
	workflow.KindConfig{Workers: 2, Remember: true, Retry: &confirmedRetry})   // skipped-slot codes retry here
finalized := workflow.Define(w, "block.finalized", fetchFinalizedBlocks,
	workflow.KindConfig{Workers: 2, Remember: true, Retry: &finalizedRetry})   // and are terminal here
statuses := workflow.Define(w, "statuses", fetchStatuses,
	workflow.KindConfig{Policy: batch.Policy{MinWait: 200 * time.Millisecond, MaxItems: 256}, Remember: true, Retry: &statusRetry})

// per notification (slot s, signature sig), on the reader goroutine:
confirmed.Submit(ctx, slotKey(s), s)                                     // Remember: one fetch per slot per run
finalized.After(slotKey(s), []workflow.Dep{root.At(s)}, constant(s))
statuses.After(sigKey(sig), []workflow.Dep{root.At(s)}, constant(sig))
// per slot notification: root.Advance(n.Root)
```

What this reproduces: eligibility instants, a late slot starting its own
clock, budgets from readiness, terminal-only-for-finalized via per-kind
classifiers, one fetch per slot per run, whole-group byte-identical retry
when nothing in the group was answered, and the in-flight drain on abort.
What it changes: batch assembly is by `MinWait`, not by slot order and
first-seen order, so the journaled request envelopes differ; a batch's
budget is per task, not anchored at its earliest signature; signature dedup
is per run, not per slot; today's cap of two in flight *across* the three
kinds, with confirmed fetches taking capacity before batches before
finalized fetches, is lost: `Workers: 2` on three kinds is six in flight
with no priority, so the in-flight cap, the five-per-second limiter and the
priority all move into the RPC client; the normal end of the request loop
is abort (abandon in-flight attempts), not drain, so the consumer
distinguishes duration expiry from SIGINT itself. `Submit` on the reader
goroutine must never block, so `MaxOpen` stays unset (unbounded). What
stays in ShitQuant: the RPC calls, the classifiers, and journaling every
request and response before the scheduler learns anything.

Net effect, counted by the review: roughly 280 lines removed from the
consumer, half of them comments pinning rulings whose tests would be
rewritten, against a new dependency on the irreplaceable path.
Recommendation: do not adopt here.

### 6.2 The normalizer's pending state (`internal/solana/normalizer.go`)

```go
blocks := workflow.NewPromise[blockArrival](w, "block.confirmed")     // block plus its record's receipt and (capture_id, sequence)
admit  := workflow.Define(w, "admit", admitFacts, workflow.KindConfig{Policy: batch.Policy{MaxItems: 1}}) // Workers 1

// per notification record (reader goroutine, record order):
for _, sw := range decode(rec) {                                       // decode stays on the reader; it is cheap
	admit.After(swapKey(sw), []workflow.Dep{blocks.Handle(slotKey(sw.Slot))}, pair(sw, blocks))
}
// per confirmed getBlock record:
if !blocks.Resolve(slotKey(b.Slot), arrival) {                          // first block stands
	if prior, _ := blocks.Handle(slotKey(b.Slot)).Result(ctx); prior.Hash != b.Hash { count(block_conflict) }
}
```

Within the admit kind this is the swap-block join with today's semantics: a
swap admitted when its block arrives, a late swap admitted at once, a
second block ignored and counted only when its hash differs, `time_unknown`
as `pair` returning a typed error the observer counts, and readiness order
equal to completing-record order: one `Resolve` is one event that readies
the held swaps in registration order, which is notification-record order
and swap order within a record.

It cannot be adopted for the join alone. The admit handler runs on the
kind's worker while the reader goroutine keeps admitting coverage, finality
and retraction facts and keeps the finality and coverage maps. Today the
block record admits the held swaps and *then* closes coverage extents, on
one goroutine; split across two, the trade admissions and the coverage
admission race for the recorder's lock, sequence order becomes
schedule-dependent, and acceptance (1) fails in exactly the way "sequence
assigned at one serialized point, never by whichever goroutine received
the message" forbids. The normalizer's maps would need a lock for the same
reason. The only correct shape puts every record kind through the one
single-worker kind as dependency-free tasks in record order, which places
the whole normalizer inside the handler: a channel to one goroutine, which
is what the live follower already is.

The finality maps (evidence read before the occurrence exists, most of it
about transactions never admitted) and coverage's event-time coalescing are
not joins the library should own either: they would be pending tasks
forever, reported as incomplete at close, where a map entry costs nothing.
And the live snapshot's "blocks awaiting swaps" count is a set of resolved
promise keys nobody registered against, which `Stats` does not expose.

Decode in parallel is possible with a sequenced step (`decode` with
`Workers: N` feeding a `Sequence`d single-worker kind that registers the
admit tasks), but nothing has measured decode as a bottleneck.
Recommendation: expressible, not adoptable in part, and not worth adopting
whole.

### 6.3 The recorder (`internal/marketdata/recorder.go`)

Revision 1 said nothing changes inside and group commit becomes one integer.
Wrong: the index assigns a sequence per mutation under the lock and persists
before applying, so a group commit is a transactional index API that
validates N, persists N in one transaction, and applies N or none. That is
work inside `marketdata`, not a batcher in front of `Admit`. The commit
latency measurement already exists; when it asks, the change is there.

### 6.4 Phase 5 flush and phase 6 runners

The one-second Valkey flush is `batch.Batcher[CandleUpdate]` with
`MaxWait: 1s`, if the ticker-based first version measures badly; the phase
outline says ticker first. Phase 6 has no design yet: its outline says
rules read a frozen view at saved cutoffs and record cutoffs and decisions
through the admission-log path, and the backtest runner drives the
historical view with no wall clock. Whether that is a fan-out of
evaluations into one single-worker admit kind is for that design document
to decide. It is the only remaining candidate for the workflow package in
ShitQuant, on a rebuildable path, and only after the design exists.

## 7. Deliberately absent

- Shared capacity with priorities across kinds (non-goal).
- Memoization beyond `Remember`, persistence, checkpoints, distribution.
- Byte-bounded queues; `WithQueue` counts items. The obvious shape if a
  measurement asks is `WithCost(func(T) int, limit)`.
- `Then`/`Join`/`Each`, `WithOrdered`, `OnError`, per-task deadlines,
  `FromChan`: each was in revision 1 and each was either unimplementable as
  specified, a duplicate of another mechanism, or a five-line loop.
- A `Processor` interface, item IDs, interfaces for testability.

## 8. Migration from v0.5

| v0.5 | v0.6 |
|---|---|
| `batch.New[T](cfg)` + `Go(ctx, src, procs...)` + `Done()` | `batch.New(handler, batch.WithPolicy(p))` + `Run(ctx)` + `Close()` |
| `Source[T].Read(ctx) (<-chan T, <-chan error)` | `iter.Seq2[T, error]` + `batch.Consume` |
| `Processor[T].Process(ctx, []*Item[T]) ([]*Item[T], error)` | `batch.Handler[T]`; chain by calling functions |
| `Item[T]{ID, Data, Error}` | `T`; per-item outcomes are the handler's |
| error channel, `IgnoreErrors`, `CollectErrors` | `Run`'s return value |
| `Config`, `ConstantConfig`, `DynamicConfig` | `Policy` and `SetPolicy` |
| `BufferConfig` | `WithQueue` |
| `RunBatchAndWait`, `ExecuteBatches` | `errgroup` |
| `processor.*`, `source.*` | a loop |
| (roadmap) sync-like batching | `batch.Loader` |
| (none) | `workflow` |

## 9. Delivery plan

1. `batch`: `Policy`, `Batcher`, `Consume`, `GroupError`, `Loader`. Tests
   under `synctest`: policy priority table, greedy zero policy, drain vs
   abort with two contexts, `Add` after `Run` returned, backpressure, worker
   bound, `SetPolicy` re-evaluation, unanswered calls, `Do` with an ended ctx,
   race detector throughout.
2. `workflow`: `Define`, `Submit`, `After`, `Promise`, `Watermark`,
   `Sequence`, `Remember`, `MaxOpen`, `Retry`, `Fatal`, `Stats`, observer.
   Tests: diamond, dependency failure propagation, dedup pending and
   remembered, promise resolved before and after registration, watermark
   order, readiness-order determinism from one goroutine, sequence with
   settle and out-of-order readiness, group retry of the unanswered
   remainder, `MaxOpen` counting descendants, `IncompleteError` at close,
   handler calling `After` under a full queue, abort with blocked handles.
3. Delete `processor` and `source`; rewrite README, `doc.go`, CHANGELOG
   (0.6.0), examples; Go 1.25 in `go.mod` and CI.
4. In ShitQuant: nothing until a measurement asks; then 6.4; the workflow
   package only if the phase 6 design calls for it.

## 10. Remaining open questions

1. `Sequence` requires the consumer to `Settle`. Is that acceptable, or is
   "settle when the submitting goroutine's `Submit` for ordinal n+1 arrives"
   a safe implicit rule for the single-submitter case?
2. Should `Loader` and `Consume` stay in `batch` (they are under fifty lines
   each) or move to keep `batch` at one type?
3. `Key string` versus a per-kind comparable type parameter.
4. Whether the `workflow` package should ship in 0.6 at all, or after it has
   carried phase 6 in ShitQuant. The library is generic and the request for
   it is real; the consumer evidence for it today is thin.
