# GoBatch redesign proposal: batching and workflows

Status: revision 3, after two rounds of three independent reviews
(adversarial, consumer fit, Go API design). The reviews and what changed because of them are in
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

Groups are handed to workers in release order and a group's slice is in
queue order; that is the only ordering promise, and with `WithWorkers(1)`
it is total. `Add` before `Run` on a batcher that is closed without ever
running returns only on ctx.

```go
// GroupError is what Run returns when a handler failed.
type GroupError struct {
	Seq uint64 // release sequence of the group, from 1
	Err error
}
func (e *GroupError) Unwrap() error

// Group describes the group a handler is serving. Attempt and Partial are
// always 1 and false in a Batcher; a workflow kind sets them on retry.
type Group struct{ Seq uint64; Attempt int; Partial bool }
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

This is the roadmap's "sync-like batching". The zero policy with the default
single worker is what a dataloader does: while the worker is busy,
everything that arrives coalesces into the next call. With more workers,
light load gives one call per `Do`; that is the deliberate default.

## 5. Package `workflow`

### 5.1 Model

A **workflow** owns **kinds**, **promises**, **watermarks** and **sequences**.

- A **kind** is a typed unit of work `In -> Out` executed by a handler over
  groups of ready tasks, with its own `batch.Policy`, worker count, retry
  policy and memory.
- A **task** is one instance of a kind, identified by `(kind, key)`, with a
  set of dependencies and a **root**: the `Submit` call it descends from.
- A **promise** is a keyed, complete-once value the consumer completes from
  outside. It is what a task depends on when the thing it waits for is not
  work but an arrival: "the confirmed block for slot S, whenever its record
  is read, or never".
- A **watermark** is a monotone counter tasks can wait on: "the chain root
  reached slot S".
- A **sequence** is a consumer-assigned total order tasks can be released in.
- A **handle** is a future for a task or a promise key. Handles, watermark
  positions and sequence positions are all `Dep`s.

A task is **ready** when every dependency is complete. Ready tasks are
released to their kind in **readiness order**, defined precisely: every
event that can complete a dependency (`Complete` on a task, `Complete` on a
promise key, `Advance` on a watermark, a sequence position becoming
releasable, a registration whose dependencies are already complete, a retry
backoff expiring) takes the scheduler lock, and a task's ready position is
assigned under that lock in the order the lock was acquired; when one event
readies several tasks, they are readied in registration order. Nothing else
orders anything. Completion satisfies dependents.

Fan-out is one handle with several dependents. Fan-in is one task with
several dependencies. A pipeline is the case where every task has one.

```
             ┌─────────────┐
             │ Workflow    │  task table, dependency edges, promises,
             │  scheduler  │  watermarks, sequences, readiness order, retry
             └──────┬──────┘
                    │  ready tasks -> the kind's release engine
       ┌────────────┼────────────┐
       ▼            ▼            ▼
   Decode        Metadata      Prices        three kinds (or promises, when the
       │            │            │           data arrives instead of being computed)
       └────────────┼────────────┘
                    ▼
                 Persist                     a kind whose tasks depend on all three
```

Each kind runs its own release engine: the same policy engine `batch.Batcher`
is built on, shared through an internal package. `workflow` does not drive
a `batch.Batcher` through its public API, because a retried group must be
re-injected as one unit with its original sequence number, which `Add`
cannot express. Policy timers in a kind run from the task's readiness
instant, not from when a worker pulled it.

### 5.2 Defining kinds

```go
type Workflow struct{ /* unexported */ }
func New(opts ...Option) *Workflow
func WithObserver(f func(Event)) Option
func WithMaxOpen(n int) Option   // roots not yet finished; 0 means unbounded (5.5)

// Run runs the scheduler and every kind until Close drains everything or ctx
// ends. Panics if called twice.
func (w *Workflow) Run(ctx context.Context) error
// Close stops registrations. Idempotent, non-blocking, safe before Run.
func (w *Workflow) Close()
func (w *Workflow) Stats() Stats

type Stats struct {
	Kinds    map[string]KindStats    // Pending, Ready, InFlight, Retrying, Done, Failed, Retried, Remembered
	Promises map[string]PromiseStats // Pending, Completed, Unreferenced
	Open     int                     // roots not yet finished
}

type KindConfig struct {
	Policy   batch.Policy
	Workers  int          // 0 means 1
	Retry    *RetryPolicy // nil: no retry
	Remember bool         // finished keys stay deduplicated, with their result, until Forget (5.3)
}

type Kind[In, Out any] struct{ /* unexported */ }
func Define[In, Out any](w *Workflow, name string, h Handler[In, Out], cfg KindConfig) *Kind[In, Out]

// Handler serves one group of ready tasks, in release order. Tasks
// unfinished when it returns are failed with the returned error, or with
// ErrUnanswered if it returned nil. Unlike batch.Handler, an error does not
// stop the run: it fails or retries the group's tasks (5.7). Fatal does.
type Handler[In, Out any] func(ctx context.Context, tasks []*Task[In, Out]) error

type Task[In, Out any] struct {
	Key     Key
	In      In
	Attempt int // this task's attempt, from 1; the group's own attempt is batch.GroupFromContext
}
func (t *Task[In, Out]) Complete(out Out) bool // true if this answer won; later answers are ignored and counted
func (t *Task[In, Out]) Fail(err error) bool
func (t *Task[In, Out]) Handle() Handle[Out]    // for registering continuations from inside the handler
```

### 5.3 Registering and depending

```go
type Key string

// Handle is a future. The zero Handle is never done. A Handle is a Dep.
type Handle[Out any] struct{ /* unexported */ }
func (h Handle[Out]) Result(ctx context.Context) (Out, error)
func (h Handle[Out]) Done() <-chan struct{}

// Submit registers a root task. deps may hold watermark and sequence
// positions and promise handles. If a task with this key is known, Submit
// returns its handle at once. Otherwise it blocks while MaxOpen roots are
// unfinished, returning ctx.Err() or ErrClosed instead. Never call it inside
// a handler.
func (k *Kind[In, Out]) Submit(ctx context.Context, key Key, in In, deps ...Dep) (Handle[Out], error)

// After registers a continuation. deps must contain at least one task
// handle (a promise or watermark alone makes a root, and roots use Submit);
// the task joins the root of every task handle in deps. build runs in this
// kind's worker once every dep is complete, immediately before the handler;
// Result on a dep returns at once there, and an error from build fails the
// task. After never blocks and may be called inside a handler, where
// t.Handle() is the usual dependency.
func (k *Kind[In, Out]) After(key Key, deps []Dep, build func(ctx context.Context) (In, error)) Handle[Out]

// Forget drops a remembered finished key so it may run again.
func (k *Kind[In, Out]) Forget(key Key)
```

Every registration dedups by `(kind, key)`: a second registration while the
first is unfinished returns the first's handle and keeps the first `in`,
before any `MaxOpen` wait. With `Remember`, finished keys dedup too, and
their results are retained, until `Forget`; the bound is the number of
distinct keys, and the consumer that knows a key is done calls `Forget`.
Without it, a finished key resubmitted runs again; the window between
finishing and resubmitting is inherent.

A dependency failing terminally fails its dependents with `*DependencyError`
without running them. Handles own results: a handle can gain dependents
after completion, and they are ready at once. A task whose root has already
finished reopens that root for the count in 5.5.

Verified with the toolchain: `Define`, `Submit` and `After` infer every type
parameter; making `Submit` and `After` methods on `*Kind` is what lets an
interface-typed `In` accept a concrete argument. `NewPromise` needs its type
argument spelled.

### 5.4 Promises, watermarks, sequences

```go
// Promise is a keyed, complete-once value completed from outside.
type Promise[Out any] struct{ /* unexported */ }
func NewPromise[Out any](w *Workflow, name string) *Promise[Out]

// Complete completes key. The first answer wins; later ones are ignored and
// counted, and return false. Never blocks; safe from any goroutine,
// including a handler. Returns false after Close.
func (p *Promise[Out]) Complete(key Key, out Out) bool
func (p *Promise[Out]) Fail(key Key, err error) bool

// Handle returns the handle for key, registering it if unseen. A completed
// key and its value are retained until Forget.
func (p *Promise[Out]) Handle(key Key) Handle[Out]
func (p *Promise[Out]) Forget(key Key)

// Watermark is a monotone counter with waiters. Usable without a workflow.
type Watermark struct{ /* unexported */ }
func NewWatermark() *Watermark
func (m *Watermark) Advance(v uint64)                          // never decreases
func (m *Watermark) Value() uint64
func (m *Watermark) Wait(ctx context.Context, v uint64) error  // Value() >= v, or ctx.Err()
func (m *Watermark) At(v uint64) Dep

// Sequence is a consumer-assigned total order (5.6).
type Sequence struct{ /* unexported */ }
func NewSequence() *Sequence
func (s *Sequence) Settle(n uint64) // every position <= n has been registered; monotone
func (s *Sequence) At(n uint64) Dep // panics if n was already used or n <= the settled mark
```

A promise is what a "pending map" is: swaps waiting for their block are tasks
depending on `blocks.Handle(slotKey)`; a block arriving before any swap is
`Complete` on a key nobody has asked for yet, counted as unreferenced in
`Stats` until something registers against it or the consumer forgets it. A
key never completed by `Close` fails its dependents (5.9). A promise's
memory is every key completed or asked for and not forgotten; the consumer
owns that bound.

A watermark is "the world advanced to v". Its only operation is `Advance`,
so the library never invents progress. When one `Advance` readies several
waiters they are readied in registration order, not by position. `Wait` is
the generation-swap broadcast a journal follower hand-writes; it does not
model "closed". A watermark is not bound to a workflow; its waiter list is
registered and copied outside the scheduler lock, and the documented lock
order is watermark before scheduler, never the reverse.

### 5.5 Bounds

Continuations (`After`, `Complete`, `Advance`) never block: a handler
blocking on a queue it feeds while holding a worker is a deadlock, and
revision 1 had one in the scheduler itself. The only backpressure point is
root `Submit`. A root is **open** until it and every task that joined it
through `After` are finished (done, terminally failed, or swept at close);
a fan-in task joins every distinct root among its task-handle deps.
`WithMaxOpen(n)` blocks `Submit` while `n` roots are open. Memory is
bounded by `MaxOpen` times the largest subtree a root produces; the library
does not cap fan-out. A root whose descendant waits on a promise that is
never completed stays open for the run; the consumer's escape is a timeout
of its own that calls `Fail` on the key. Ready queues are unbounded, so the
scheduler never blocks on a kind.

### 5.6 Release order

Default: readiness order (5.1). It is deterministic when every event that
completes dependencies is deterministic: `Complete`, `Advance` and `Submit`
issued from one goroutine in a fixed order, and kinds with `Workers: 1`
whose own inputs were deterministic. It is not deterministic across kinds
with several workers, and the library says so rather than pretending a
reorder buffer can fix it. It is determinism of *release order*: group
boundaries depend on `MinWait`/`MaxWait` and are time-dependent, so a
handler whose output depends on group composition is not replay-stable
unless `MaxItems` is 1.

Explicit: a task with `seq.At(n)` among its deps is **releasable** once it is
ready, every position below `n` has been released or terminally failed,
and `Settle(m)` has been called for some `m >= n`, so nothing smaller can
still appear. Released means moved from the sequence buffer to the kind's
ready queue, a scheduler-side event independent of worker timing. Ties
between distinct keys at one position release in registration order. A
retried sequenced task re-enters the ready queue directly; the position was
consumed by its first release. The consumer knows when a record's tasks are
all registered; the library does not, so the consumer calls `Settle`. The
cost is head-of-line blocking: a task whose dependency never arrives blocks
everything behind it, so a sequenced task must depend only on things that
always finish, which a computation does and an arrival does not.

### 5.7 Retry

```go
type RetryPolicy struct {
	Attempts int                  // total, including the first
	Budget   time.Duration        // from the task's first readiness; checked before each re-invocation, so the last attempt may run past it
	Backoff  func(attempt int) time.Duration
	Retry    func(err error) bool // nil: everything; never sees a Fatal error
}
```

One rule, on the handler: a handler error applies to every unfinished task
in the group. If the kind has a policy and `Retry(err)` is true and attempts
and budget remain, those tasks enter **retrying**: their handles stay
unresolved and their dependents are untouched, no worker is held, and after
`Backoff` the unfinished remainder is re-invoked as one group with the same
`batch.Group.Seq`, `Attempt+1` and `Partial` true if anything in the
original group was completed. Otherwise they fail terminally with
`*RetryError`, which resolves handles and fails dependents. Tasks the
handler already completed are never re-sent. A task failed with `Fail(err)`
is retried alone under the same policy, in a fresh group. The observer
reports `Task.Attempt`.

A terminal task failure is data: it fails handles and dependents, and `Run`
goes on. The only way a handler ends the run is `Fatal(err)`: returned from
the handler, from `build`, or passed to `Fail`. It bypasses the classifier,
fails the group's unfinished tasks with the unwrapped `err`, aborts the
workflow and makes `Run` return `err`. `errors.Is(Fatal(err), ErrFatal)`
holds. Failures that must latch (a persist error) are `Fatal`; failures that
are outcomes (an RPC budget exhausted) are not. `batch.Loader` has no
`Fatal`: there, any handler error already stops.

### 5.8 Rules for handlers

Stated once, in the `Handler` doc, and checked in the race tests:

- Never call `Submit` or `Watermark.Wait` inside a handler; each can block
  on the scheduler the handler is part of.
- `Result` inside a handler or `build` returns at once on the current task's
  own deps; on any other handle it can block a worker, so do not.
- `After`, `Complete`, `Fail`, `Advance` are always safe.
- Answer every task before returning. The worker closes each task under its
  lock before failing the unanswered ones, so a late answer from a goroutine
  the handler left behind is ignored and counted, deterministically.

### 5.9 Lifecycle

- `Run` returns when `Close` has been called and nothing is ready, in
  flight, building or retrying; or when ctx ends. Before returning it fails,
  in one critical section with the closed flag, every task still pending on
  a promise, watermark, sequence or unfinished dependency with
  `*IncompleteError`, which lists the directly blocked tasks as
  `{Kind, Key, Waiting []Dep}`; their dependents fail with
  `*DependencyError` wrapping it. Every handle resolves. If tasks were
  pending, `Run` returns that error.
- After `Close`, `Submit` returns `ErrClosed`; `After` returns a non-zero
  handle already failed with `ErrClosed`; promise `Complete`/`Fail` return
  false; `Advance` still advances the watermark (it is not bound to the
  workflow) but readies nothing.
- Abort fails every pending task with ctx.Err() and waits for in-flight
  handlers.
- Panics propagate as in 4.2.

Task state machine, with the goroutine that drives each transition:

| From | To | Driven by |
|---|---|---|
| registered | ready | scheduler lock holder: the goroutine calling `Complete`/`Advance`/`Settle`, the registering goroutine when deps are already complete, or the backoff timer |
| registered | sequenced | scheduler, when ready but its sequence position is not yet releasable |
| sequenced | ready | scheduler, when the position becomes releasable |
| ready | building | the kind's worker |
| building | in handler | the kind's worker (`build` ok) |
| building | failed | the kind's worker (`build` error); `Fatal` aborts instead |
| in handler | done | `Complete` (handler goroutine) |
| in handler | retrying | handler error or `Fail` classified retryable, with attempts and budget left (handler goroutine) |
| in handler | failed | handler error, `Fail`, `ErrUnanswered` otherwise (handler goroutine); `Fatal` aborts instead |
| retrying | ready | backoff timer |
| registered/sequenced/ready | failed | `Run` return (`*IncompleteError` or ctx.Err()); terminal failure of a dependency |

A group all of whose `build`s failed is not invoked.

### 5.10 Observation

```go
type Event struct {
	Seq     uint64    // transition order, assigned under the scheduler lock
	Kind    string
	Key     Key
	Type    EventType // Registered, Ready, Started, Completed, Retrying, Failed
	Attempt int
	Err     error
}
```

`WithObserver(func(Event))` is called outside the scheduler lock, on the
goroutine that made the transition, so an observer may call `Complete` or
`After` safely; observed order can differ from transition order, which is
why `Seq` exists. `Stats` is a snapshot.

### 5.11 Errors

```go
type DependencyError struct{ Kind string; Key Key; Err error } // Unwrap
type TaskError       struct{ Kind string; Key Key; Err error } // what Result returns for a failed task; Unwrap
type RetryError      struct{ Attempts int; Err error }         // Unwrap
type IncompleteError struct{ Pending []PendingTask }
type PendingTask     struct{ Kind string; Key Key; Waiting []Dep } // Dep has String
var ErrClosed     = batch.ErrClosed
var ErrUnanswered = errors.New("workflow: task not answered by handler")
var ErrFatal      = errors.New("workflow: fatal")
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

// per notification (slot s, signature sig), on the reader goroutine; all roots, MaxOpen unset:
confirmed.Submit(ctx, slotKey(s), s)                                     // Remember: one fetch per slot per run
finalized.Submit(ctx, slotKey(s), s, root.At(s))
statuses.Submit(ctx, sigKey(sig), sig, root.At(s))
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
	admit.Submit(ctx, swapKey(sw), sw, blocks.Handle(slotKey(sw.Slot)))  // a root: MaxOpen unset, never blocks
}
// per confirmed getBlock record:
if !blocks.Complete(slotKey(b.Slot), arrival) {                         // first block stands
	if prior, _ := blocks.Handle(slotKey(b.Slot)).Result(ctx); prior.Hash != b.Hash { count(block_conflict) }
}
```

Within the admit kind this is the swap-block join with today's semantics: a
swap admitted when its block arrives, a late swap admitted at once, a
second block ignored and counted only when its hash differs, `time_unknown`
as `pair` returning a typed error the observer counts, and readiness order
equal to completing-record order: one `Complete` is one event that readies
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
The live snapshot's "blocks awaiting swaps" is `Stats.Promises` unreferenced
count, and every completed block is retained until the consumer `Forget`s
the slot.

Decode in parallel is possible with a sequenced step (`decode` with
`Workers: N`, each admit registration carrying `seq.At(recordSequence)` and
the reader calling `Settle` per record), but nothing has measured decode as
a bottleneck.
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
2. Release 0.6.0 with `batch` alone: delete `processor` and `source`;
   rewrite README, `doc.go`, CHANGELOG, examples; Go 1.25 in `go.mod` and CI.
3. `workflow`, as 0.7.0: `Define`, `Submit`, `After`, `Promise`, `Watermark`,
   `Sequence`, `Remember`/`Forget`, `MaxOpen`, `Retry`, `Fatal`, `Stats`,
   observer. Tests: diamond, dependency failure propagation, dedup pending
   and remembered, promise completed before and after registration,
   watermark order, readiness-order determinism from one goroutine,
   sequence with settle, out-of-order readiness and a terminal failure
   ahead in the buffer, retrying keeps handles unresolved, group retry of
   the unanswered remainder with `Partial`, `MaxOpen` counting roots through
   fan-in and handler-side `After`, `IncompleteError` at close atomic with a
   racing `After`, abort with blocked handles, late answers ignored.
4. In ShitQuant: nothing until a measurement asks; then 6.4; the workflow
   package only if the phase 6 design calls for it.

## 10. Remaining open questions

1. `Key string` versus a per-kind comparable type parameter. Kept as
   `string`: `Dep`, `Event` and `PendingTask` would otherwise need a type
   parameter each.
2. `workflow` ships as 0.7 after `batch` as 0.6, so nothing irreplaceable
   depends on an unproven scheduler. Whether 0.7 waits for ShitQuant's phase
   6 design to exercise it, or ships on its own tests, is the maintainer's
   call; the request for it is real and the consumer evidence today is thin.
3. `Remember` retains results as well as keys. If a kind only needs
   once-per-key without the result, a cheaper `RememberKeys` mode may be
   worth adding after a measurement.
