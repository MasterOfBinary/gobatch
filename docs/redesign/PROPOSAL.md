# GoBatch redesign: batching, request coalescing, and in-process flow

Status: revision 1 of a new lineage (2026-09-20). A from-scratch redesign of
GoBatch with workflow-style fan-out. Independent subagent reviews (adversarial,
consumer-fit, Go API) are recorded under [reviews/](reviews/) as they land.
[REVIEWS.md](REVIEWS.md) lists what changed because of them.

This is not a copy of `origin/claude/library-redesign-workflow-9kjqu1`. That
branch designed a dynamic keyed-task scheduler (`workflow` with Promise,
Watermark, Sequence, Remember, retry). Two review rounds made that scheduler
*expressible* and then showed ShitQuant should not adopt it on the paths that
matter. Official issues `#97`–`#100` already chose a smaller thing: a finite
in-process graph named `flow/`, replacing an earlier plan to use OnyxCore.

This document takes that product decision as binding, keeps the batching
primitive that survived those reviews, and adds only the concurrency
composition the user's diagram actually needs.

```
             ┌─────────────┐
             │ Flow runner │   one input, finite DAG, bounded
             └──────┬──────┘
                    │
       ┌────────────┼────────────┐
       ▼            ▼            ▼
   Decode        Metadata      Prices     shared Loaders (process lifetime)
   Loader         Loader        Loader
       │            │            │
       └────────────┼────────────┘
                    ▼
                 Persist
                 Loader or stream Batcher
```

Private consumer repos (`shitquant`, `onyxcore`, `shitlock`) were not readable
from this environment. ShitQuant claims use the public gobatch issues plus a
prior code-backed review of that tree. OnyxCore and shitlock are inferred and
marked.

## 1. What the current library gets wrong

v0.5 is a single `Batch[T]` that reads one `Source[T]`, groups by `Config`, and
runs each group through a chain of `Processor[T]`. The window math is the
product. Everything around it fights real use.

| Problem | Consequence |
|---|---|
| One type parameter for the whole pipeline | A stage cannot change type; multi-stage work falls back to `Batch[any]` |
| `Source.Read` returns two channels | Every source re-implements goroutine, close, and cancellation; a nil channel is a runtime error |
| Errors are a side channel that must be drained | An undrained `errs` deadlocks once the buffer fills; `IgnoreErrors` exists to paper over the API |
| Per-item `*Item[T]` with an engine-assigned ID | One allocation per item; the ID is not a dedup key, an order key, or a correlation ID |
| Each ready batch starts an unbounded goroutine | No in-flight cap; under load this is a fork bomb with a queue in front |
| Collector ignores context (`waitForItems` takes `_`) | Cancel does not stop formation; drain and abort are the same word |
| `Config` is an interface for four integers | `DynamicConfig` exists only because there is no `SetPolicy` |
| Single-use `Batch` with `Go` / `Done` / error channel | Three-step lifecycle where `Run(ctx)` plus `Close()` would do |
| Request/reply batching is not expressible | N callers into one bulk call cannot be built on a push stream |
| No fan-out, fan-in, or bounded parallel | Consumers hand-write `errgroup` *or* invent a hub; the official answer is a small `flow/` |
| Silent `fixConfig` mutation | `MinItems: 0` becomes 1 with no error; invalid limits are rewritten |
| `WithBufferConfig` panics after start | Misuse crashes the process instead of returning an error |
| Documented Go 1.18, CI on 1.25/1.26 | The support floor is an untested claim |
| Master is ahead of the latest tag | README documents a `Go` signature `go get` does not compile |

Worth keeping: min/max items, min/max wait, and their priority. The rest is
replaced.

## 2. Three approaches

### A. Harden v0.5 and bolt on new packages

Keep `Source`, `Processor`, `Item`, `Go`/`Done`/error channel. Add
`RequestBatcher` and `flow/` beside them. Land bounds, drain/abort, and a
pin-able tag so ShitQuant can depend on today's shape.

- For: smallest migration; matches the letter of issues `#73`, `#95`, `#96`.
- Against: the structural smells stay (must-drain channels, one `T`, unbounded
  process goroutines unless patched in place). Two APIs forever. A "redesign
  from scratch" that is not one.

### B. Layered redesign (recommended)

Three independently usable pieces, one lifecycle vocabulary:

1. `batch.Batcher[T]` — stream window, `Run`/`Add`/`Close`, bounded workers.
2. `batch.Loader[In, Out]` — request/reply coalescing, one outcome per `Do`.
3. `flow` — compile a finite DAG; sequence, bounded parallel, fan-in,
   conditions; nodes are ordinary functions that may call shared Loaders.

Delete `source` and `processor`. A source is an iterator or a loop around
`Add`. A processor is a function.

- For: matches `#71`, `#97`–`#100` and the user's diagram; hard to misuse;
  does not become a hub; leaves OnyxCore room to sit above later.
- Against: breaking. ShitQuant cannot pin this until it ships. The current
  master still needs an honesty tag so `go get` matches the README.

### C. Dynamic keyed workflow (previous redesign lineage)

`Batcher` + `Loader` + a scheduler of keyed tasks, promises, watermarks,
sequences, Remember, and retry — the revision-3 `workflow` package.

- For: can *express* a pending map and a Helius-style scheduler.
- Against: a prior consumer-fit review showed it would change admission
  order, land on the irreplaceable capture path, and recreate the archived
  actor/hub/quota system. `#98` forbids dynamic graphs, automatic retries,
  and a scheduler in v1. Encroaches on OnyxCore. High implementation risk
  for a consumer that has a house rule: batching only after a measurement.

**Recommendation: B.** Ship A-style honesty (tag, changelog, docs) in
parallel so today's master is pin-able, then break toward B.

## 3. Goals and non-goals

Goals:

1. A stream batcher that is hard to misuse: `Run`, `Add`, `Close`; named
   drain and abort; bounded workers and queue; policy you can replace.
2. Request/reply batching: N concurrent `Do` calls, one handler, each caller
   one terminal outcome. No implicit key coalescing.
3. A small in-process flow runner for one input: validated finite graph,
   sequence, bounded parallel, fan-in, conditions, explicit outcomes.
4. Shared Loaders across concurrent flow runs, without the runner owning or
   closing them.
5. One observation vocabulary. No telemetry backend.
6. Every rule statable in one godoc sentence. No goroutine the user did not
   ask for.

Non-goals (library):

- Durable or distributed workflows, job databases, cron, brokers, workers, DLQ.
- Dynamic keyed-task scheduling, promises, watermarks, reorder buffers.
- Implicit coalescing, generic retries, priority across kinds.
- Byte-accurate memory limits for arbitrary `T`.
- A DSL, YAML graph, plugin/type registry, or UI.
- Owning durability, time, or order. Those stay in the application.
- Depending on OnyxCore, Redis, or a lock library.

## 4. Shape

```
github.com/MasterOfBinary/gobatch/batch    Batcher[T], Loader[In, Out], Policy
github.com/MasterOfBinary/gobatch/flow     Graph, Runner, Sequence, Parallel, Map
```

`flow` depends on nothing in `batch`. They compose at the application:
a node calls `Loader.Do`. `batch` depends only on the standard library.

Go floor **1.25**, matching the oldest toolchain CI already tests. That gives
`iter` for sources, 1.23+ timer `Reset` semantics, and `testing/synctest` for
every wait test. The documented 1.18 floor is dropped in the same change.

`processor` and `source` are deleted.

Functional options are all `With…`. Invalid options fail at `New` / `Compile`
with an error, never a panic, never silent rewrite.

## 5. Package `batch`

### 5.1 Policy

```go
// Policy decides when the pending group is released to a handler.
//
// A group is released as soon as any of these holds:
//   - MaxItems > 0 && len(group) >= MaxItems
//   - MaxWait  > 0 && MaxWait has elapsed since the first item was queued
//   - len(group) >= MinItems && MinWait has elapsed since the first item
//   - Flush or Close was called and the group is non-empty
//
// MinItems below 1 is 1. Zero MaxItems or MaxWait means no maximum. A max
// smaller than its min is rejected at New or SetPolicy, not clamped.
//
// The zero Policy is greedy: a group is released as soon as one item is
// queued and a worker is free, and it takes everything queued at that
// moment. MaxItems 1 is therefore one item per handler call.
//
// MaxWait and MinWait are measured from the first item of the group. An
// idle batcher never fires an empty timer.
type Policy struct {
	MinItems int
	MaxItems int
	MinWait  time.Duration
	MaxWait  time.Duration
}
```

`SetPolicy` re-evaluates the pending group under the new policy. It cannot
raise `MaxItems` above the queue bound or disable safety limits.

This is a formation policy, not an end-to-end deadline and not backpressure.
Backpressure is a full queue: `Add` blocks.

### 5.2 Batcher

```go
// Handler processes one released group. The slice is the handler's after
// the call. Returning an error stops the batcher: Run returns a *GroupError
// and in-flight handlers see a cancelled context. A handler that wants to
// continue after a failure handles it and returns nil.
type Handler[T any] func(ctx context.Context, items []T) error

func New[T any](h Handler[T], opts ...Option) (*Batcher[T], error)

// Run executes until Close drains it or ctx ends. It returns nil after a
// drain, ctx.Err() after an abort, or the *GroupError that stopped it.
// Run panics if called twice. On any return, every in-flight handler has
// returned, and Add returns ErrClosed from then on.
func (b *Batcher[T]) Run(ctx context.Context) error

// Add queues one item. It blocks while the queue is full. It returns
// ctx.Err() or ErrClosed instead of queuing; when both apply, ctx.Err()
// wins. Add before Run queues up to the queue size and then blocks until
// Run starts or ctx ends. Add after Run has returned returns ErrClosed.
func (b *Batcher[T]) Add(ctx context.Context, item T) error

func (b *Batcher[T]) Flush()  // release the pending group now; never blocks
func (b *Batcher[T]) Close()  // stop accepting; Run drains. Idempotent.
func (b *Batcher[T]) SetPolicy(p Policy) error
func (b *Batcher[T]) Stats() Stats // Queued, InFlight, Groups, Items; a snapshot

func WithPolicy(p Policy) Option
func WithWorkers(n int) Option // concurrent handler calls; default 1; n < 1 is an error
func WithQueue(n int) Option   // items Add may queue before blocking; default 1024; n < 1 is an error
func WithObserver(f func(Event)) Option
```

Groups go to workers in release order. A group's slice is in queue order.
That is the only ordering promise. With `WithWorkers(1)` it is total.

Lifecycle, two words:

- **Close** means drain: everything accepted is processed.
- **Context cancellation** means abort: queued-not-started items are
  dropped, blocked `Add` calls return, in-flight handlers see a cancelled
  context and are waited for. A handler that must finish a side effect
  already in flight does so under `context.WithoutCancel`. A deadline is
  never evidence that a write failed.

A consumer that wants "stop reading at the deadline, then drain what was
read" uses two contexts:

```go
g, runCtx := errgroup.WithContext(cleanupCtx) // cleanup: abort
b, err := batch.New(persist, batch.WithPolicy(batch.Policy{MaxItems: 500, MaxWait: 50 * time.Millisecond}))
g.Go(func() error { return b.Run(runCtx) })
g.Go(func() error {
	defer b.Close()
	return batch.Consume(readCtx, b, records(readCtx)) // read deadline: stop reading
})
return g.Wait()
```

Panics in handlers are recovered in the worker and re-raised from `Run`, as
`errgroup` does, so the stack is the handler's and the process still dies.

```go
type GroupError struct {
	Seq uint64 // release sequence, from 1
	Err error
}

func (e *GroupError) Unwrap() error

type Group struct {
	Seq     uint64
	Attempt int  // always 1 on a Batcher
	Partial bool // always false on a Batcher
}

func GroupFromContext(ctx context.Context) (Group, bool)

var ErrClosed = errors.New("batch: closed")
```

### 5.3 Sources

```go
// Consume adds every item of seq to b and returns the first error from seq
// or Add. It cannot interrupt seq between yields, only the Add. It does
// not Close b.
func Consume[T any](ctx context.Context, b *Batcher[T], seq iter.Seq2[T, error]) error
```

`iter.Seq2[T, error]` is the whole source contract. A channel is a loop
around `Add`. No channel adapter is exported.

### 5.4 Loader: request/reply batching

This is the roadmap's "sync-like batching" and issue `#71`.

A Loader is not a Batcher with a reply slot bolted on. Two rules differ:

1. A lone caller must not wait for `MinItems` or `MinWait`. Those fields
   are rejected at `NewLoader` unless `MinItems <= 1` and `MinWait == 0`.
   Release is `MaxItems`, `MaxWait` (linger), Flush, or Close.
2. Every accepted `Do` gets exactly one terminal outcome. A missing handler
   answer is `ErrUnanswered`, never a zero `Out`.

```go
type Call[In, Out any] struct {
	In In
	// unexported identity and reply slot
}

func (c *Call[In, Out]) Complete(out Out) bool // first answer wins; later ignored and counted
func (c *Call[In, Out]) Fail(err error) bool

// LoadHandler serves one group of calls. Calls unanswered when it returns
// are failed with the returned error, or with ErrUnanswered if it returned
// nil. The handler must not assume calls[i] corresponds to a particular
// map entry; it correlates by the Call value and Call.In.
type LoadHandler[In, Out any] func(ctx context.Context, calls []*Call[In, Out]) error

func NewLoader[In, Out any](h LoadHandler[In, Out], opts ...Option) (*Loader[In, Out], error)
func (l *Loader[In, Out]) Run(ctx context.Context) error
func (l *Loader[In, Out]) Close()
func (l *Loader[In, Out]) Flush()
func (l *Loader[In, Out]) SetPolicy(p Policy) error
func (l *Loader[In, Out]) Stats() Stats

// Do queues in and waits for its reply. Equal In values are distinct
// operations; there is no implicit coalescing. If ctx ends first, Do
// returns ctx.Err(); the call stays in its group and its reply is
// discarded, so the caller does not know whether the handler acted on
// it. A caller that must know uses a context that does not end.
//
// Do after Close or after Run has returned returns ErrClosed.
func (l *Loader[In, Out]) Do(ctx context.Context, in In) (Out, error)
```

The handler context is a lifecycle context with a finite timeout (option
`WithHandlerTimeout`; default 30s, 0 means none). One caller's cancel does
not cancel that context and does not fail sibling `Do`s in the same group.

The zero policy with the default single worker is the dataloader shape:
while the worker is busy, arrivals coalesce into the next call; when the
worker is free, the first `Do` releases immediately (no MinWait). With more
workers, light load gives one call per `Do`. That is the deliberate default.

Shutdown: `Close` then `Run` returns after every accepted `Do` has a
terminal outcome. Abort (`Run`'s ctx) fails not-yet-started calls with
`ctx.Err()`, cancels the handler context, and waits for in-flight handlers
up to a graceful budget (`WithShutdownBudget`, default 10s). Past the
budget, `Run` returns a `*ShutdownError` listing still-running handler
groups; capacity is not released until those handlers return. Blocked `Do`
waiters are failed when their call is settled, not before.

```go
var ErrUnanswered = errors.New("batch: call not answered by handler")

type ShutdownError struct {
	Pending int
	Err     error // usually context.DeadlineExceeded
}
```

Attribution: extra `Complete`/`Fail` after the first are ignored. Duplicate
or unsolicited answers cannot invent a new `Do`. The library never turns a
missing result into success.

## 6. Package `flow`

### 6.1 Model

A **graph** is an immutable, validated DAG compiled from named nodes.
A **run** is one execution of a graph against one input.
A **runner** admits a bounded number of concurrent runs and a bounded
number of executing nodes across runs.

- A node function runs **at most once** per run, after every declared
  dependency has succeeded.
- Failed or skipped prerequisites skip descendants. The skip reason is
  recorded. Independent branches finish unless the run is cancelled.
- There is no optional-edge engine, no dynamic `After` from inside a node,
  no retry.

Fan-out of known cardinality is `Map` (a helper) or several sibling nodes.
Fan-out of unknown cardinality is a node body that calls `Map` or
`Loader.Do` in a loop. Fan-in is declared dependencies.

The user's diagram is three sibling nodes then a persist node, with each
I/O node calling a process-lifetime Loader.

### 6.2 Building a graph

The application owns a typed envelope. Nodes receive the same `In` (almost
always a pointer) and write disjoint fields. The library cannot enforce
field disjointness; a data race is a bug in the graph, and `-race` will
see it.

```go
type Node[In any] func(ctx context.Context, in In) error

type Predicate[In any] func(ctx context.Context, in In) (bool, error)

type Builder[In any] struct{ /* unexported */ }

func New[In any]() *Builder[In]

// Node adds a task. deps are node IDs that must succeed first.
// Duplicate IDs, unknown deps, or a cycle fail at Compile, not at Run.
func (b *Builder[In]) Node(id string, fn Node[In], deps ...string) *Builder[In]

// If evaluates pred once after deps succeed.
//   - pred error  → this node Failed; descendants Skipped (dependency failed)
//   - pred false  → this node Succeeded with false; descendants Skipped (condition)
//   - pred true   → this node Succeeded with true; descendants may run
// False is not a failure. There is no Else edge; write a second If.
func (b *Builder[In]) If(id string, pred Predicate[In], deps ...string) *Builder[In]

func (b *Builder[In]) Compile() (*Graph[In], error)
```

`Compile` rejects: empty graph, empty ID, duplicate ID, unknown dependency,
self-dependency, cycle, more nodes than 256. The 256 cap is a compile-time
constant on the builder (`DefaultMaxGraphNodes`). A runner whose
`WithMaxGraphNodes(n)` is smaller than `len(g.Nodes())` rejects at `Run`
with a validation error and does not admit the run. `Compile` does not run
anything. The graph is immutable and reusable.

```go
type Graph[In any] struct{ /* unexported */ }

func (g *Graph[In]) Nodes() []string // stable compile order
```

### 6.3 Running

```go
type Runner struct{ /* unexported */ }

func NewRunner(opts ...RunnerOption) (*Runner, error)
func WithMaxConcurrentRuns(n int) RunnerOption // default 1; n < 1 is an error
func WithMaxExecutingNodes(n int) RunnerOption // default 8; n < 1 is an error
func WithMaxGraphNodes(n int) RunnerOption     // checked at Compile via Runner.Compile, or Builder default
func WithObserver(f func(Event)) RunnerOption
func WithShutdownBudget(d time.Duration) RunnerOption

// Run admits one execution. It rejects with ErrSaturated if the run bound
// is full; it does not queue waiting runs. It returns after every started
// node function has returned.
func Run[In any](ctx context.Context, r *Runner, g *Graph[In], in In) (*Result, error)

// Close stops admission. In-flight runs are not cancelled by Close; cancel
// their contexts. Close is idempotent. The runner does not close Loaders
// or other caller-owned adapters.
func (r *Runner) Close()

type Status int

const (
	StatusSuccess Status = iota
	StatusFailed
	StatusSkipped
	StatusCanceled
)

type NodeOutcome struct {
	Status Status
	Err    error // set on Failed; skip/cancel reason on Skipped/Canceled
}

type Result struct {
	Status Status
	Nodes  map[string]NodeOutcome
}

func (res *Result) Outcome(id string) (NodeOutcome, bool)
```

`Run`'s error is `ctx.Err()` on cancel after admission, `*GraphError` if
any node failed (the run still fills `Result`), `ErrSaturated` if rejected,
or `ErrClosed` if the runner is closed. A failed node does not cancel
independent branches. Cancel prevents new dispatch and cancels the context
seen by running nodes.

Panic in a node or predicate is recovered to that node's Failed outcome
(`ErrPanic`); scheduler capacity is released. The process does not die.
This differs from `batch`, where a handler panic is a programming error
that must surface from `Run`. A flow node is user business logic; a
batch handler is a tight loop the library cannot continue past.

### 6.4 Helpers that are not a graph

These are `errgroup` with names, for one-shot code that does not need
Compile. They share the cancel rule (started functions are waited for)
and nothing else. They do not skip, do not produce `Result`, and do not
go through a Runner.

```go
func Sequence[In any](ctx context.Context, in In, fns ...Node[In]) error
func Parallel[In any](ctx context.Context, in In, limit int, fns ...Node[In]) error

// Map runs fn over items with at most limit in flight. limit < 1 is an
// error. The first fn error cancels the rest and is returned after they
// finish. Items is a snapshot; Map does not see later appends.
func Map[T any](ctx context.Context, limit int, items []T, fn func(context.Context, T) error) error
```

`Each` that returns `[]Handle` without knowing the count is not provided.
Unknown cardinality is a node that calls `Map` after it has the slice.

### 6.5 Composition with batch

```go
type Work struct {
	Token   Token
	Decoded Decoded
	Meta    Metadata
	Prices  []Price
}

decodeL, _ := batch.NewLoader(decodeBulk, batch.WithPolicy(batch.Policy{MaxItems: 32, MaxWait: 20 * time.Millisecond}))
metaL, _ := batch.NewLoader(metaBulk, batch.WithPolicy(batch.Policy{MaxItems: 32, MaxWait: 20 * time.Millisecond}))
priceL, _ := batch.NewLoader(priceBulk, batch.WithPolicy(batch.Policy{MaxItems: 32, MaxWait: 20 * time.Millisecond}))
persistL, _ := batch.NewLoader(persistBulk, batch.WithPolicy(batch.Policy{MaxItems: 16, MaxWait: 50 * time.Millisecond}))

// Run each Loader for process lifetime. The flow runner does not own them.

g, err := flow.New[*Work]().
	Node("decode", func(ctx context.Context, w *Work) error {
		d, err := decodeL.Do(ctx, w.Token)
		w.Decoded = d
		return err
	}).
	Node("metadata", func(ctx context.Context, w *Work) error {
		m, err := metaL.Do(ctx, w.Token)
		w.Meta = m
		return err
	}).
	Node("prices", func(ctx context.Context, w *Work) error {
		p, err := priceL.Do(ctx, w.Token)
		w.Prices = p
		return err
	}).
	Node("persist", func(ctx context.Context, w *Work) error {
		_, err := persistL.Do(ctx, *w)
		return err
	}, "decode", "metadata", "prices").
	Compile()

res, err := flow.Run(ctx, runner, g, &Work{Token: tok})
```

Decode, metadata, and prices start together. Persist waits for all three.
Each `Do` may sit in a group with `Do`s from other concurrent `Run`s.

Shutdown order: cancel or finish runs, `Runner.Close`, then `Loader.Close`
on each shared loader. Reversing that deadlocks waiters or returns
`ErrClosed` from inside a still-running node (which is Failed, not a
library panic).

A stream `Batcher` is the persist path only when the work is one-way and a
measurement asked for a time/size window (ShitQuant phase-5 flush). Persist
that must correlate a reply uses a Loader.

## 7. Shared contracts

### 7.1 Observation

```go
type Event struct {
	Seq   uint64
	Name  string    // kind, loader, or node id
	Type  EventType // Admitted, Released, Started, Completed, Failed, Skipped, Canceled
	Size  int       // group size, or 1 for a node
	Err   error
	Cause string    // formation cause: max_items, max_wait, min_wait, flush, close
}

func WithObserver(f func(Event)) Option
```

The callback runs outside internal locks, on the goroutine that made the
transition. It must not block. A panic in the observer is recovered and
counted. Payloads are never included. There is no internal event queue:
a slow observer delays that transition's caller, not an unbounded buffer.

`Stats` is a snapshot: queued, in flight, released groups, completed,
failed. No promise that two fields were sampled at the same instant
beyond "one mutex hold."

### 7.2 What is deliberately absent

- Shared capacity with priorities across loaders or nodes.
- Memoization, persistence, checkpoints, distribution.
- `Then` / `Join` / `Each` returning handles, `FromChan`, per-task
  deadlines, `OnError` callbacks.
- A `Processor` interface, item IDs, interfaces invented for mocks.
- `Promise`, `Watermark`, `Sequence` (the workflow kind). A monotone
  counter the app needs is `uint64` plus a `cond`. A pending map the app
  needs is a map.

## 8. Consumer mapping

### 8.1 ShitQuant (high confidence)

Ground: public issues `#71`, `#73`, `#75`, `#95`–`#100`, and a prior
code-backed review of `internal/helius/scheduler.go`,
`internal/solana/normalizer.go`, `internal/marketdata/recorder.go`,
`internal/capture/journal.go`. This environment could not open the private
repo. House rules from that review: batching only after a measurement;
never manufacture time or order; a deadline is never evidence a write
failed; cancellation is not a drain; do not recreate the archived hub.

| Path | Use gobatch? | How |
|---|---|---|
| Capture journal | No | Irreplaceable bytes. Pause, do not drop. `Add` abort drops queued items. |
| Helius scheduler | No | Per-kind retry, forever dedup, slot-ordered batches, 2 in-flight / ~5/s live in the app. A Loader would change journaled envelopes. |
| Normalizer pending maps | No | Network-free, one goroutine, completing-record order. A library join reorders or races the recorder lock. |
| Recorder / group commit | No | Durable-before-visible is a transactional `Memory` API, not a Batcher in front of `Admit`. |
| Enrichment / proposal | **Yes** | `flow` graph vs the direct-call/replay baseline (`#97`–`#99`). |
| Multi-token outcome sampling | **Yes** | Shared `Loader`s (`#71`, `#100`). |
| Phase-5 Valkey flush | **Yes, after measurement** | `Batcher` with `MaxWait: 1s` if a ticker measures badly. |
| Paper / replay | Flow only if the graph is the enrichment path | Replay clock and order stay in the app. |

Adoption order: pin a release, use Loader+flow on enrichment, measure the
flush, never put this library on capture.

### 8.2 OnyxCore (medium confidence)

Issue `#97`: build `flow/` in GoBatch, replacing an earlier recommendation
to use OnyxCore. Do not delete or archive OnyxCore; do not depend on it.

Public signals: 2023 forks of `imgui-node-editor`, `imnodes`, `imgui`,
`glfw`. "If I ever pick it up again." The most plausible reading is a
richer node-graph host, possibly visual, not a batcher.

Leave room: stable node IDs, explicit outcomes, observation events a UI
could display, compiled graphs that do not require gobatch types inside
node functions.

Do not absorb: dynamic graphs, named-port schemas, plugin registries,
ImGui, cron, a job database, distributed workers. Those are why OnyxCore
still exists.

`cutgraph` is a public Python video-timeline repo. It does not add
requirements here.

### 8.3 Shitlock (low confidence)

No public repo, issue, or commit was found. Three guesses, all implying
gobatch should take nothing:

| Guess | Need from gobatch | Offer to a future distributed gobatch |
|---|---|---|
| Distributed / Redis lock (owner has `redistypes`, `gobatch-redis`) | None | Fencing/lease API that a worker plane could call. That plane is still not gobatch. |
| Trading / position lock next to ShitQuant | None | None |
| Process / single-writer lock for the journal | None. Must not hide inside `Run`. | None |

Do not add `gobatch/lock`. If a lock library appears, it stays a dependency
of the application or of a future plane, not a gobatch package.

### 8.4 Capability matrix

| Capability | ShitQuant | OnyxCore | Shitlock | Future |
|---|---|---|---|---|
| Stream Batcher | Useful after measurement; harmful on capture | Weakly useful | Unused | Required |
| Loader | Required off capture | Useful as a callee | Unused | Required |
| Static flow | Required for enrichment | Useful as a callee; must not replace OnyxCore | Unused | Required |
| Dynamic keyed workflow | Harmful | OnyxCore-owned if it exists | Unknown | Out |
| Promises / watermarks | Harmful as a library runtime | Unknown | Unused | Out |
| Generic retry | Harmful | Out of `flow/` | Lock-library owned | Out |
| Durable jobs | App-owned | OnyxCore-owned if any | Unused | Out |
| Locks | App-owned | Unused | Provides them | Out |
| Observation | Useful | Useful for a UI | Unused | Required as hooks |
| Bounds | Required | Useful if it embeds | Own wait bound | Required |

### 8.5 What must stay out

Capture journal, normalizer maps, recorder sequencing, Helius scheduling,
provider credentials, Redis (`#72`), retries, implicit coalescing, a lock
API, OnyxCore types, estimated-memory trackers, a root-package rename as a
blocker for this work.

## 9. Migration from v0.5

| v0.5 | Redesign |
|---|---|
| `batch.New[T](cfg)` + `Go` + `Done` | `batch.New(handler, WithPolicy(p))` + `Run` + `Close` |
| `Source[T].Read` | `iter.Seq2[T, error]` + `Consume`, or a loop around `Add` |
| `Processor[T].Process` | `Handler[T]`; chain by calling functions |
| `Item[T]{ID, Data, Error}` | `T`; per-item outcomes are the handler's |
| error channel, `IgnoreErrors`, `CollectErrors` | `Run`'s return value |
| `Config`, `ConstantConfig`, `DynamicConfig` | `Policy` and `SetPolicy` |
| `BufferConfig` | `WithQueue` |
| `RunBatchAndWait`, `ExecuteBatches` | `errgroup` |
| `processor.*`, `source.*` | a loop |
| (roadmap) sync-like batching | `Loader` |
| (none) | `flow` |

No compatibility shims. This is v0. A tagged v0.5.x honesty release of
*current* master is the shim: applications that are not ready to move keep
a pin.

## 10. Production roadmap

Two tracks. Track 0 does not wait for the redesign. The redesign does not
wait to be perfect before Track 0 tags.

### Track 0 — honesty (days of work, current API)

Ship a tag that matches master so `go get` compiles the README.

1. Fill `[Unreleased]` for `#64` and `#66` (`#93`).
2. Tag (v0.5.1 or v0.6.0 of the *current* surface — maintainer call).
3. State the real Go floor: raise `go.mod` to 1.25 or add a 1.18 CI leg
   (`#85`). This redesign assumes the raise.
4. Reconcile or close stale PRs `#65`, `#67`, `#68`, `#76` so they do not
   land contradictory lifecycles (`#95`).
5. Verify an external module with `GOWORK=off` and no `replace`.

This is the pin ShitQuant can take while the redesign is built.

### Track 1 — `batch` v0.7.0 (the new primitive)

`Policy`, `Batcher`, `Consume`, `Loader`, `GroupError`, observer, bounds.
Delete `processor` and `source`. Rewrite README, `doc.go`, CHANGELOG,
examples, AGENTS.md / CLAUDE.md.

Tests under `synctest`: policy priority table, greedy zero policy, invalid
policy rejected, drain vs abort with two contexts, `Add` after `Run`
returned, full-queue backpressure, worker bound, `SetPolicy` re-evaluation,
unanswered calls, `Do` with an ended ctx, equal `In` is two operations,
caller cancel does not cancel siblings, shutdown budget,
late `Complete` ignored, race detector throughout.

### Track 2 — `flow` v0.8.0

`Builder`, `Compile`, `Runner`, `Run`, `If`, `Sequence`, `Parallel`, `Map`.
Examples: ShitQuant-shaped enrichment (fixtures, no paid APIs) and a
non-trading metadata/classification graph (`#98`, `#99`).

Tests: diamond, cycle/unknown/duplicate rejection, condition false vs
error, skipped descendants, independent branch completion, saturation
(`ErrSaturated`, no waiting-run queue), cancel at each state, panic
recovery, concurrent runs isolated, shared Loader across runs (`#100`)
as a follow-on once both packages exist, admission/close races,
non-cooperative node termination reporting.

`#100` does not block a standalone flow release (`#99`).

### Track 3 — v1.0

Freeze after:

- Track 0 pin exists and a consumer compiled against it.
- Track 1 and 2 have soak tests and a second consumer example.
- Documented support floor, drain/abort, bounds, and observation are
  implemented as specified, not as coverage percentages.
- No open contradiction between README and code.

v1.0 may still be a 0.x mentally; the point is a pin with a migration
guide and a promise that *this* surface is the one to break next on
purpose, not by accident.

### Track 4 — later, not this redesign

- Redis adapter (`#72`) after a named consumer needs it.
- Optional key coalescing on Loader, behind an explicit option, after
  `#71`'s distinct-identity default has shipped.
- Dynamic keyed workflow / Promise / Watermark — OnyxCore or a new
  proposal with a measurement.
- Visual graph editor — OnyxCore.
- Byte-cost queues (`WithCost`) after a measurement.
- Daemon example and idle-soak test on the *new* `Run` loop.

### Production-readiness checklist (library, not a platform)

| Item | When |
|---|---|
| Tag matches docs | Track 0 |
| Go floor tested | Track 0 / 1 |
| No must-drain channels | Track 1 |
| Named drain vs abort | Track 1 |
| Bounded workers and queues | Track 1 |
| Invalid config is an error | Track 1 |
| One outcome per accepted `Do` | Track 1 |
| Observation hooks | Track 1–2 |
| Validated finite graphs | Track 2 |
| Bounded runs and executing nodes | Track 2 |
| Shared Loader + flow example | Track 2 follow-on (`#100`) |
| External pin with `GOWORK=off` | Track 0 and again at v1 |
| Race tests, synctest waits | every track |
| CI permissions, no-op cache, lint once | Track 0 (`#85`) |
| No panic-on-misuse public API | Track 1 |

## 11. Open questions

1. **Honesty tag number.** v0.5.1 (patch of current) vs v0.6.0 (current
   surface, because `#64` already broke `Go`). Maintainer call; the
   redesign then becomes v0.7/v0.8 as above.
2. **`Key` coalescing.** Out of v1. If added later, it is
   `WithCoalesce(func(In) K)` and default remains distinct identities.
3. **Envelope vs returned values.** Pointer envelope is the v1 graph
   model. A type-changing pipeline is two graphs or ordinary Go in a node.
4. **Whether `flow.Run` is a package function or `Runner.Run`.** Package
   function needs a type parameter; method does not infer `In` as nicely.
   Draft uses the package function. Revisit if inference is ugly in
   compiled stubs.
5. **Handler timeout default.** 30s is a guess. 0 (none) is safer for
   journal-like handlers and worse for a stuck RPC. Prefer 0 if a
   reviewer shows a case where a default timeout invents failure.

## 12. Difference from the previous redesign branch

| Previous (`workflow` lineage) | This proposal |
|---|---|
| Dynamic keyed tasks, Promise, Watermark, Sequence, Remember, retry | Finite DAG, no promises, no retry |
| Scheduler as the product | Window + Loader + small graph |
| Recommended replacing Helius / joining in the normalizer | Explicitly forbids both |
| `Each` / `After` / handle graphs | `Map` + declared deps |
| Shared priority `Pool` | Out (starves; app wraps the client) |
| One `workflow` package | `batch` then `flow`, independently usable |
| Go 1.25, `Run`/`Add`/`Close`, Loader, Policy | Kept; those parts survived review |

The previous branch remains useful as a record of dead ends. Do not merge
it. Do not implement `workflow` because a document already exists.
