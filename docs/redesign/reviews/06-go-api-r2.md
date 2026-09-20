# Second API pass: PROPOSAL.md revision 2

Read in full. Every signature in sections 4 and 5, plus the section 6 call
sites (including `constant(...)`, interface-typed `In` through `After`'s
build, `admit.Ordinal(1).After(...)`, `blocks.Handle(...)`, `Fatal`,
`GroupFromContext`), was compiled as a stub module with go1.24.7
(`scratchpad/infer2/`). Result: everything compiles; there are no
type/function name reuses, no methods with extra type parameters, and the
value-receiver `Handle` satisfies the unexported-method `Dep`. `NewPromise`
needs its type argument spelled (`workflow.NewPromise[Block](w, ...)`)
because nothing infers `Out`; that is unavoidable and the example already
does it. The problems below are semantic.

## Findings, ranked

### 1. `After` outside a handler escapes `MaxOpen`; the 6.1 example is unbounded

5.5 says the only backpressure point is root `Submit`, and "each task
carries its root". A task registered with `After` whose deps are only a
watermark or promise handle has no root. 6.1 registers two such tasks per
notification from the reader goroutine (`finalized.After(... root.At(s) ...)`,
`statuses.After(... root.At(s) ...)`), so the bound the section claims does
not exist there. The split the design wants is "root, counted, may block"
versus "continuation, uncounted, never blocks", and that is not the same as
"has deps" versus "has none". Make it the signature:

```go
// Submit registers a root task. It counts against MaxOpen and blocks past it.
// deps may be watermarks, promise handles or task handles. Never call it in a handler.
func (k *Kind[In, Out]) Submit(ctx context.Context, key Key, in In, deps ...Dep) (Handle[Out], error)

// After registers a continuation of the task whose handle is the first task
// handle in deps; it inherits that root. It never blocks. It panics if deps
// hold no task handle: a task with no ancestor is a root, and roots use Submit.
func (k *Kind[In, Out]) After(key Key, deps []Dep, build func(ctx context.Context) (In, error)) Handle[Out]
```

This also deletes the `constant(s)` helper 6.1 needs today, since a root's
input is a value: `finalized.Submit(ctx, slotKey(s), s, root.At(s))`.

### 2. `Kind.Ordinal(n)` returning a view is a smell; make the ordinal a `Dep`

The doc describes it two ways at once ("sets the ordinal of the task
registered by the next Submit/After" is stateful; "returns a view; the kind
itself is unchanged" is not), a stored view can be reused for two
registrations with one ordinal, it allocates a `*Kind` per registration, and
it is the only per-registration datum that travels on the kind instead of
the call. The `slog.With` precedent covers derived configuration, not
per-call arguments.

The idiomatic alternative is already in the design, one line above:
`Watermark.At(v) Dep`. An ordinal is a readiness condition ("every smaller
ordinal has been released and nothing smaller can still appear"), so:

```go
type Sequence struct{ /* unexported */ }
func NewSequence() *Sequence
func (s *Sequence) Settle(n uint64)   // every ordinal <= n has been registered
func (s *Sequence) At(n uint64) Dep   // satisfied once ordinals < n are released and n is settled; panics on reuse of n
```

Used as `admit.Submit(ctx, key, in, seq.At(n))` or in an `After` deps list.
`KindConfig.Sequence` and `Kind.Ordinal` go away, the "registration without
one panics" rule goes away (a kind is sequenced per task, by having the dep),
head-of-line blocking is visible in the deps list, and `Sequence` becomes a
standalone primitive like `Watermark`. Keep the `Workers: 1` advice in its
doc. If a per-call option is preferred instead, a variadic `...TaskOption`
with `Ordinal(n)` is the fallback, but revision 2 rightly removed option
families and this would bring one back; `SubmitAt`/`AfterAt` doubles the
entry points for one field and I would not do it.

Open question 1 (implicit settle): no. A record can yield ordinals n..n+k in
any order, so "n+1 registered" does not mean "n settled". The consumer knows
record boundaries; keep `Settle` explicit.

### 3. Three completion vocabularies again

`Task.Complete/Fail` (no return), `Call.Complete/Fail` (no return),
`Promise.Resolve/Reject` (bool). Revision 1's note on `Reply` vs `Complete`
applies to `Resolve`/`Reject`, which are JavaScript's words. Use
`Complete`/`Fail` on all three, and return `bool` from all three: the doc
already says "first answer wins; later ones are ignored and counted", and
`bool` for "did this call win" is `atomic.CompareAndSwap`/`Mutex.TryLock`
shape. Handlers will ignore it; tests and the 6.2 `block_conflict` counter
will not.

`Promise.Handle(key)` is fine: a method named like a type is legal
(verified) and reads naturally at the call site; registering on first read is
`sync.Map.LoadOrStore` shape. Two things it must say: a resolved value is
retained for the run so a later `After` can find it, which makes a promise's
memory the number of distinct keys ever resolved (6.2 resolves one per slot,
unbounded over a long run); and what `Close` reports for it. `PendingTask`
lists tasks, so a promise key with a handle but no dependent task is not
reported. Add `func (p *Promise[Out]) Forget(key Key)` for the consumer that
knows a slot is done, and put resolved-but-unreferenced counts in `Stats`.

### 4. `Fatal(err)` is fine; make it detectable and say where it is honoured

The closest precedents are `fs.SkipDir`/`fs.SkipAll` (a sentinel returned to
change the walker's control flow) and `http.ErrAbortHandler` (a sentinel the
handler panics with). A wrapper is better than a sentinel here because the
consumer wants both "stop" and "which error", so keep `Fatal`, but export the
marker so an observer, a test or a wrapping handler can see it:

```go
var ErrFatal = errors.New("workflow: fatal")
func Fatal(err error) error   // errors.Is(Fatal(err), ErrFatal) and errors.Is(Fatal(err), err) both hold
```

State that `Run` returns `err` unwrapped as the doc says, and whether `Fatal`
is honoured from `build`, from `Fail(Fatal(err))`, and from a `LoadHandler`
(there is no `Fatal` in `batch`, where any error already stops). Also state,
in both `Handler` docs, that the two handler types have opposite defaults:
`batch.Handler` error stops the batcher, `workflow.Handler` error fails the
group's tasks and the run continues.

### 5. Textual contradictions to fix before code

- 5.3 "The zero Handle is never done" versus 5.9 "After ... no-ops that
  return zero handles already failed". A zero handle cannot be failed. Return
  a non-zero handle already failed with `ErrClosed`.
- 5.3 `Result`: "never call inside a handler" versus `After`'s build "may call
  Result on the deps", which runs on the same worker goroutine. The rule is
  about which handles, not where: "Result on a completed dependency of the
  current task returns at once; waiting on any other handle from a worker can
  deadlock the scheduler." Say that in 5.8.
- 5.2 `Task.Attempt` versus `batch.Group.Attempt` from the context: one is
  the task's own retry count, the other the group's. Name the difference in
  both docs.
- 5.9: a task pending on a task that is pending on a promise gets
  `*DependencyError` wrapping the `*IncompleteError`, not `*IncompleteError`
  directly; say so, or `PendingTask` should list transitive dependents too.

### 6. `After` argument order

`(key, deps, build)` is right: Go puts the function last (`sort.Slice`,
`slices.SortFunc`, `context.AfterFunc`, `errgroup.Go`), so `deps` cannot be
variadic without moving a func literal into the middle of the call. Keep the
slice; with finding 1, `After` always has at least one dep anyway, and
`Submit` gets the variadic form because its `in` is a value.

### Smaller

- `KindConfig.Remember` reads as a verb; `RememberKeys bool` or `Memo bool`.
  Not insisting.
- `Workflow.Stats()` mentions per-kind fields but no `Stats` type is shown;
  it needs `Kinds map[string]KindStats` plus per-promise counts (finding 3).
- Open question 2: keep `Loader` and `Consume` in `batch`. Question 3: keep
  `Key string`. Question 4: ship `batch` as 0.6 now; ship `workflow` when
  phase 6 has a design document that uses it, as its own 0.7, so the
  irreplaceable path never depends on an unproven scheduler.

## The five changes I would insist on now

1. `Submit(ctx, key, in, deps ...Dep)` as the only outside-handler entry
   point and the only thing `MaxOpen` counts; `After` requires a task-handle
   dep and panics otherwise (finding 1).
2. Replace `Kind.Ordinal` and `KindConfig.Sequence` with `Sequence.At(n) Dep`
   (finding 2).
3. `Complete`/`Fail` returning `bool` on `Task`, `Call` and `Promise`; no
   `Resolve`/`Reject` (finding 3).
4. State promise memory (retained for the run), add `Promise.Forget`, and
   define what `Close` reports for unresolved promise keys (finding 3).
5. Export `ErrFatal`, and fix the zero-handle and `Result`-in-handler
   contradictions in 5.3/5.8/5.9 (findings 4, 5).
