# Review: does the GoBatch redesign proposal fit its one consumer?

Reviewed: `/home/user/gobatch/docs/redesign/PROPOSAL.md` against the ShitQuant tree at `/home/user/shitquant` (CLAUDE.md, DESIGN.md, docs/design/normalization.md, docs/design/raw-capture.md, internal/helius/{scheduler,request,source}.go, internal/solana/{normalizer,finality,coverage}.go, internal/marketdata/{recorder,memory}.go, internal/capture/{journal,reader}.go, cmd/shitquant/live.go).

Short verdict: the `batch` package is a reasonable library on its own terms, but section 6 does not describe ShitQuant. Six of the seven "mapped" claims are wrong on inspection, two of them in ways that would break the consumer's stated invariants, and the proposal's own code examples do not do what the prose says they do. The maintainer, under DESIGN.md's rules, should not adopt the `workflow` package for ShitQuant at all, and should adopt `batch` only for the phase-5 Valkey flush and only after that measurement.

Findings are ranked by how much they change the design.

---

## 1. Ranked findings

### F1. Section 6.2's ordering model inverts ShitQuant's admission order and can stall the live view (design-breaking)

Proposal 5.6: "A continuation inherits the ordinal of its first dependency ... The ordinal is fixed at registration and never depends on when anything completed." Applied to `Join2(admit, key, swapHandle, blk, build)` with `InOrder()`, a swap's admit ordinal is its *notification* record's position.

ShitQuant admits in *completing-record* order. `normalizer.go:386-436` (`block`) admits a slot's held swaps when the confirmed block record is read; `normalizer.go:345-384` (`swap`) admits a late swap when its own record is read. Admission order is therefore "whichever half arrived second", and the replay availability stamp is defined as exactly that record's `receipt_time` (`normalization.md:15`, `:112`; `recorder.go:388-392`, `CompletingReceipt`).

Concrete divergence. Slot A: swap at record 50, block at record 200. Slot B: swap at record 60, block at record 70.
- Today: B's trade admitted at record 70, A's at 200. Log order B, A. Replay availability B = receipt(70), A = receipt(200).
- Proposal with `InOrder`: task 50 (A) has the lower ordinal and is unready until record 200, so task 60 (B) "behind an unready one waits" (5.6). Log order A, B; and because availability is clamped non-decreasing (`recorder.go:404-406`, `memory.go:526-528`), B's replay availability becomes receipt(200), not receipt(70).

That is not "a few milliseconds more optimistic" (`normalization.md:15`); it is a different dataset with a different meaning of `available_time`. In live mode it is worse: if A's confirmed block never arrives in the capture (a real case: `time_unknown`, exhausted budget, `raw-capture.md:62`), task 50 never becomes ready or failed, and the reorder buffer blocks *every* later admit for the rest of the run. The live view freezes while the capture keeps writing. Proposal 5.6 names head-of-line blocking as "the cost" but does not see that ShitQuant's join has an input that may never arrive.

The proposal's claim that this "is precisely a serialized admission point whose output must be byte-identical between a live run and a replay" (5.6) confuses *determinism* with *the right order*. Today's normalizer is already deterministic by construction (one goroutine, record order; `normalization.md:157`, `live.go:331-334`). The library adds nothing there and changes the order.

### F2. Section 6.2 misreads the normalizer as a fetcher; the join it proposes is not expressible in the API (design-breaking)

`normalization.md` 6.2 snippet, line 594: `blk := workflow.Submit(ctx, confirmed, slotKey(rec.Slot), rec.Slot)` inside the normalizer, where `confirmed` is the 6.1 kind whose handler is `fetchConfirmedBlocks` (an RPC).

The normalizer never fetches anything. It reads capture records (`normalizer.go:70-72`, `:251-287`); `normalize` is a network-free command (CLAUDE.md invariants: "The market model, the demo path and normalize have no network access"; `normalization.md:9`). The confirmed block is not a task the normalizer can run; it is a record that will be read later, or never. What the pending map holds is a *keyed promise completed from outside*: "swap for slot S waits for the block record for slot S, whichever arrives first" (`normalizer.go:112-120`).

The proposed API has no such primitive. `Submit` runs a handler. `Gate` is a single monotone `uint64` (5.4) and slot block arrival is neither single nor monotone (blocks for slots 100, 98, 103 arrive in capture order). `Handle` can only be completed by the kind's handler. So "the pending map *is* the scheduler's dependency table" (6.2) is not true of anything in section 5. To make it true the library would need `Promise[K comparable, V]` with complete-once semantics (F3 below), and then the normalizer would be calling the same map it has now through a library.

The same misread recurs for finality: `entries`/`waiting` (`finality.go:43-58`, `normalizer.go:91-95`) are "evidence read before the thing it is about exists", keyed by signature and by slot. A statuses record yields up to 256 entries (`finality.go:190-204`), most of which name transactions that will never be admitted (failed, unselected pool, `finality.go:210-212`). As workflow tasks those would be pending forever and reported in `*Incomplete` at `Close` (5.9); as map entries they cost nothing and are right.

### F3. The 6.1 example does not reproduce the scheduler; three of its lines are wrong on the proposal's own semantics (design-breaking for 6.1)

(a) **The statuses kind does not batch.** `workflow.Policy(batch.Policy{MaxItems: 256})` (line 548). Per 4.1, a group is released when `len >= MinItems && MinWait elapsed`, with `MinItems` defaulting to 1 and `MinWait` zero, so every signature is released the instant it is ready: one `getSignatureStatuses` per signature, not per 256. Today `assembleBatches` (`scheduler.go:443-496`) folds *every* pending signature from rooted slots at each 200 ms tick (`source.go:70`, `request.go:103-104`). The nearest equivalent is `MinWait: 200ms` or `MinItems: 256, MaxWait: 200ms`, and neither reproduces "in slot order and each slot's first-seen order" (`scheduler.go:469-493`), which the journaled request envelope records and inspection keys on (`request.go:154-157`, `:277-281`).

(b) **The retry policy makes a skipped-slot code terminal at confirmed commitment.** One `retry := workflow.RetryPolicy{..., Retry: isTransient}` is passed to all three kinds (lines 543-550). `outcomeFor` (`request.go:238-248`) returns `OutcomeTerminal` only for `FinalizedBlock`; the same codes at confirmed commitment must retry (`raw-capture.md:63`, final row). The example needs two policies or a kind-aware classifier; as written it would burn no quota but would mark confirmed fetches terminal that the design says to retry.

(c) **`Close` is the wrong end for the request loop.** 4.2: "Close means drain ... This is what a `--duration` cap that expires normally needs." 5.9: "Every already-submitted task still runs" through `Close`. The request loop's normal end is the opposite: "stops starting new work, abandons whatever is still in flight and drains [results], so Run never waits out a ten-second attempt" (`request.go:72-74`, `:117-132`), and a two-minute retry budget must not delay `capture_stop` (`live.go:228-234` stops the journal right after the source returns). So the consumer must use abort (ctx cancel) on the *normal* path, then distinguish `ctx.Err()`-because-duration from `ctx.Err()`-because-SIGINT itself. That is the "drain vs abort" distinction the proposal says it fixes, applied backwards for this consumer.

(d) **Group retry vs handler error is contradictory, and the resolution aborts the capture.** 5.7: a group whose handler returned an error is re-invoked. 5.9: `Run` returns "the first handler error whose kind has the stop policy (a handler error ... is a bug or an unrecoverable condition)". A status batch that exhausts three attempts is ordinary ("unresolved", `raw-capture.md:62`, `scheduler.go:344-362`), never fatal. Under 5.7+5.9 as written it is either a workflow-stopping error or requires `OnError` to swallow it, at which point the group is not retried. There is no spelled-out path that gives "retry whole group byte-identically up to N attempts within budget, then give up quietly".

(e) **Dedup is lost.** 5.3/7: "Dedup applies only while a task is pending. Re-submitting a finished key runs it again." The scheduler dedups forever: `slotFor` (`scheduler.go:500-512`) makes a second notification for a known slot a no-op for the confirmed fetch, and `seenSig` (`scheduler.go:123`, `:203-208`) drops a redelivered signature. Helius redelivers (`scheduler.go:177-181`, ruling 16). With the proposal a redelivered notification after the confirmed fetch completed issues a second confirmed fetch and a second status entry. The consumer must keep both sets, which is `slotState` minus two `requestState`s. Goal 2.5 ("dedup by first receipt") is not delivered by the mechanism in section 5.

(f) **The rate cap changes shape.** `rate.NewLimiter(5, 1)` is a token bucket with burst 1: one start per 200 ms, strictly. `hasCapacity`/`pruneStarts` (`scheduler.go:401-430`) allow five starts in any trailing one-second window, i.e. a burst of five then a pause. Not an invariant, but "reproduces today's behaviour" it is not, and the proposal names the sliding window as something that "disappears" without saying it is replaced by different semantics.

(g) **Priority within a kind changes.** `Due` walks `sortedSlots()` (`scheduler.go:254-296`), so a late-observed lower slot is offered before higher slots. `Pool` ties are FIFO (5.8). Again not an invariant, but the request order in the journal changes.

Behaviours that map cleanly: eligibility instants per request (`root.At(s)` satisfied at registration for a late slot gives it its own clock; `scheduler.go:192-200`, ruling 9/10); budget per readiness instant (5.7 vs `scheduler.go:98-108`); "terminal only for finalized" if two policies are used; `Kind+Slot` vs `BatchID` identification dissolves into `(kind, key)` plus `GroupOf(ctx)`; in-flight bound `Pool(2)`; per-attempt goroutine and the unbuffered result channel (`request.go:85-111`) replaced by workers; "evidence queued before the scheduler is told" stays consumer code either way (`request.go:163-177`, `source.go:776-819`).

### F4. "536 lines disappear" is not honest; roughly 280 net lines move, and the dependency lands on the irreplaceable path

`scheduler.go` is 536 lines, about half of them comments pinning numbered rulings. What actually goes away if the workflow package did everything section 5 says:

| Range | Lines | Fate |
|---|---|---|
| `requestState.eligible` 80-108 | 29 | gone |
| `Due` 240-299 | 60 | gone |
| `Complete` 301-342 | 42 | gone (replaced by ~10 lines of `Complete`/`Fail` in handlers) |
| `begin`/`hasCapacity`/`copySignatures`/`pruneStarts` 382-430 | 49 | gone |
| `assembleBatches` 432-496 | 65 | gone, but the ordering it guaranteed is lost (F3a) |
| `batchByID`/`sortedSlots` 514-536 | 23 | gone |
| `ObserveRoot` walk 218-238 | 21 | gone (one `Advance`) |
| `Scheduler` struct/`New` 155-175 | 21 | gone, replaced by ~15 lines of `Define`/`Pool`/`Retry` wiring |
| `request.go` `serveRequests`/`finishRequests` 75-132 | 58 | gone, replaced by `errgroup` + `Run` |
| `Unresolved` 344-380 | 37 | already has **no production caller** (`grep` finds only the definition and tests); would need an `Observer`-based rebuild if wanted |
| `slotState`/`seenSig`/`slotFor`, `ObserveTransaction` dedup | ~40 | **stays** (F3e) |
| `PendingRequest`/`RequestKind`/`Outcome`, `DefaultSchedulerConfig` | ~70 | mostly **stays**: the journal envelope needs kind, slot, signatures, attempt (`request.go:282-299`), and the mirrored constants become `RetryPolicy`/`Pool` arguments |
| `issue`/`requestRecord`/`rpcFailure`/`outcomeFor` | ~110 | **stays**, split into three handlers plus a kind-aware classifier; likely grows |

Net: about 370 lines removed, about 90 added back, so roughly 280 lines net, half of them comments. `scheduler_test.go` (780 lines) pins the rulings; those rulings still bind the capture, so the tests are rewritten against library behaviour, not deleted. In exchange ShitQuant takes a dependency of an estimated 1.5-2.5k lines (reorder buffer, gate, pool with priority, two-level retry, observer) on the capture process, the one process whose output "cannot be bought or rebuilt later" (DESIGN.md Principles). Step 4 of the delivery plan calls the scheduler the "smallest blast radius"; it is the largest. The normalizer is rebuildable from bytes; the capture is not.

### F5. 6.3 "Nothing changes inside" the recorder is false for group commit

`Memory.appendMutation` (`memory.go:515-553`) validates, assigns the sequence, calls `persist` for *that one mutation* with the lock held, and applies to the index only after the commit returns; "rejected mutations allocate no sequence" and "a persist failure leaves no state" (`normalization.md:135`, `recorder.go:420-440`). A group commit "in one bbolt transaction" (6.3) means N validations and N sequences before one commit, and a rollback of all N in-memory sequences if the commit fails. That is a new transactional `Memory` API, not "one integer". The measurement hook already exists (`Recorder.CommitLatency`, `recorder.go:501-516`, printed by `live.go:499-509`); when it asks, the change is inside `marketdata`, and a `Batcher` in front of `Admit` is neither necessary nor sufficient.

### F6. The `batch` primitive's abort semantics cannot carry the journal path

Not proposed for it explicitly, but 4.2 and 4.6 invite it. `Journal.Append` returns the commit result even after ctx ends ("once the writer owns a record it reports commit or failure regardless of ctx", `journal.go:375-377`; `raw-capture.md:113`; `source.go:369-374`). `Batcher.Add` has no per-item reply at all, and `Group.Do`'s behaviour when ctx ends after the call was queued is unspecified. If `Do` returns `ctx.Err()` for a queued call, the caller has an uncertain append, which the design forbids ("callers never retry an uncertain append"). The proposal's "a deadline is never evidence that a write failed" (4.2) is about the handler side; the caller side is where ShitQuant's rule lives.

---

## 2. Question 1: section 6.1 ruling by ruling

| Ruling / behaviour (`scheduler.go`) | Proposal | Verdict |
|---|---|---|
| Confirmed eligible at first sight (`:188-190`) | `Submit` at notification | reproduced |
| Finalized/statuses eligible at root (`:233-236`) | `root.At(s)`, readiness at `Advance` | reproduced |
| Late slot starts its own clock (`:192-200`, ruling 9/10) | dep satisfied at registration, budget from readiness | reproduced |
| Batch budget anchored at earliest signature (`:443-496`; test `:685-700`) | per-task budget + whole-group retry (5.7) | under-specified: what happens when some tasks in a group are past budget and others are not is not stated; either the group shrinks (not byte-identical) or task budgets are ignored |
| Frozen byte-identical batch, stable `BatchID` (`:129-133`, `:278-282`) | group retry, same tasks same order, `GroupOf(ctx)` | reproduced only via the handler-error path, which 5.9 also calls a stop condition (F3d) |
| `Kind+Slot` vs `BatchID` | `(kind, key)` + group id | dissolves; genuine simplification |
| Terminal only for finalized (`request.go:238-248`) | per-kind `Retry` func | reproducible; the example does it wrong (F3b) |
| Priority confirmed → batches → finalized (`:256-296`) | `Pool` priorities 0/1/2 | reproduced across kinds; within-kind slot order becomes FIFO (F3g) |
| `hasCapacity`: `<2` in flight and `<5` starts in `(now-1s, now]` (`:401-430`) | `Pool(2)` + `rate.NewLimiter(5,1)` | in-flight reproduced; rate shape changes (F3f) |
| Evidence queued before scheduler told (`source.go:776-819`, `request.go:163-177`) | consumer code | preserved if the consumer writes it that way; the library neither helps nor hurts |
| `finishRequests` drain (`request.go:117-132`) | abort waits for in-flight handlers | reproduced, but only under abort, which the proposal calls the SIGINT path (F3c) |
| Request budget per eligibility instant (`:98-108`) | `RetryPolicy.Budget` from readiness | reproduced; `Deadline` option must not be used |
| Forever dedup of slots and signatures (`:186-208`, `:500-512`) | dedup while pending only | lost (F3e) |
| Deterministic slot-ordered batch assembly (`:469-493`) | policy-driven release | lost (F3a) |
| `Unresolved` semantics: terminal counts as unresolved, attempted-and-not-done (`:344-380`) | `Stats` per kind, `*Incomplete` for gate-blocked | not derivable as specified (`Stats` has no "attempted at least once"); currently dead in production anyway |
| No blocking in observe (`:144-151`) | root `Submit` blocks under `MaxPending` | changed: the WebSocket reader would block on the scheduler; must leave `MaxPending` unset |

## 3. Question 2: section 6.2 semantic by semantic

| Normalizer semantic | Library join? | Verdict |
|---|---|---|
| block_conflict: second confirmed block ignored (`normalizer.go:399-407`) | no; dedup is only while pending, and completed handles cannot be "completed again and ignored" | consumer state |
| time_unknown: null `blockTime` (`:377-381`, `:416-424`) | `build` returning an error fails the dependents with `*DependencyError` (5.3); the count then has to be recovered via `Observer` or `Handle.Result` | consumer state plus an observer; today it is one counter increment |
| Late notification for a known slot admits at once (`:370-383`) | joining a completed handle | reproduced, *if* the block promise existed (it does not, F2) |
| Admit order market → trade → finality (`:443-536`) | inside one handler | consumer code, unchanged |
| Finality evidence read before the occurrence (`finality.go:43-58`, `:213-224`, `normalizer.go:533-535`) | would be tasks pending forever for the ~most entries that never get an occurrence | consumer state |
| Block-wide retraction in admission order (`finality.go:129-151`, `n.signatures`) | none | consumer state |
| Coverage two-sided `qualify` join (`coverage.go:236-252`) called from both `slotNotification` and `blockTime` | keyed two-sided join, not expressible | consumer state |
| 10 s coalescing (`coverage.go:24`, `:280-289`) | by *event-time span*, not wall clock or count; `Policy` has neither | consumer state; a wall-clock `MaxWait` here would manufacture time |
| `Each` keyed by "occurrence ID" (6.2, 5.5) | occurrence ID is `<chain>/<blockhash>/<sig>/<ordinal>` (`normalizer.go:478`); blockhash is unknown until the block join | misread; key must be `(sig, ordinal)` |

Net: the normalizer keeps every map it has (`normalizer.go:83-104`), gains a scheduler, and moves from "one goroutine, no lock" to "one worker plus a reader plus a ticker" with a lock (Q4). The only parallelizable stage is `ParseTransactionNotification`/`Flatten`/`Venue.Swaps` (`normalizer.go:313-330`), which is a pure function; `pumpswap.Venue` is stateless after `New` (`venue.go:36-42`, "Swaps keeps no state between calls"). If decode ever measures as the bottleneck, `errgroup` with N workers feeding one ordered channel gives the same shape in ~30 lines without any of section 5.

## 4. Question 3: invariants

| Invariant | Preserved? | Where naive use breaks it |
|---|---|---|
| Durable before visible | yes, it lives in `Recorder`/`Memory` (`memory.go:544-551`) and the proposal does not touch it | group commit (F5) if done by wrapping `Admit` rather than changing `Memory` |
| Sequence and availability assigned at one serialized point, never by the receiving goroutine | the *point* stays (`recorder.go:302-304`); the *order* does not | `Workers(n>1)` on admit; or coverage/finality admitted from the reader goroutine while trades admit on the worker: two goroutines racing for the lock, order by scheduling |
| Availability never decreases | yes, clamp in `recorder.go:404-406` and `memory.go:526-528` | cannot break, but F1 shows the clamp silently absorbing a reordering, which turns "never decreases" into "meaningless" |
| Two runs over one capture leave a byte-identical log | preserved only if *every* fact of every kind goes through the one `InOrder`, `Workers(1)` kind; 6.2 routes only swaps | any second submitter; decode workers registering continuations on completion (the proposal admits this in 5.6) |
| Cancellation is not a drain; a deadline never proves a write failed | handler side yes (4.2); caller side no (F6) | using `Batcher`/`Group` in front of `Journal.Append` or `Recorder.Admit` |
| Backpressure is pause not drop | `Add` blocks, yes; abort "drops queued items" (4.2) | using a `Batcher` for the capture queue (`source.go:920-1016` pauses on bytes and never drops; `push` under `WithoutCancel` at `:373`, `:441`); `Then`/`Join`/`Each` never block, so fan-out is unbounded (open question 2) |
| A failure latches | `Recorder.failed` (`recorder.go:430-434`) and stop policy, yes | `OnError` returning nil on a persist handler; task failures are "data" and never latch, so an admit-kind failure must be promoted by the handler |
| Never manufacture time or order | time: yes, nothing stamps from the batcher clock; order: **no** (F1: ordinals invent an admission order the evidence did not have) | `InOrder` with inherited ordinals; wall-clock `MaxWait` on anything event-time based (coverage) |
| Batching only after a measurement asks | no: 6.2's `MaxItems: 200, MaxWait: 20ms`, 6.4's `MaxWait: 1s`, 6.1's whole workflow are unmeasured; `mvp-phases.md:116` and DESIGN.md:85 make GoBatch conditional on a measurement | the proposal itself |

## 5. Question 4: single-goroutine ownership

`live.go:331-334`: "One goroutine owns the journal reader, the normalizer, the recorder and the printed snapshots: the normalizer holds its pending state without a lock and expects one reader." `normalizer.go:70-72` says the same. `runLive` relies on it at `:244-247` ("Nothing below reads the follower's state until it has stopped").

With decode on N workers and admit on one:
- Must move into task inputs/outputs: the decoded swaps *and* the per-transaction skip list (today `n.count` is called from `notification`, `normalizer.go:332-334`, on the shared `skips` map). `selected` is read-only and can be shared.
- Must become the admit worker's alone: `pending`, `markets`, `occurrences`, `signatures`, `finalized`, `waiting`, `entries`, `times`, `lastTime`, `runs`, `gap`, `records`, `through`, `skips`, `anomalies`. That forces *all* non-notification records (blocks, statuses, slot notifications, controls) to become admit tasks too, else the reader goroutine and the worker race on those maps. The reader becomes a submitter and the whole normalizer becomes the admit handler.
- Must become lock-protected: `Pending()` and `Summary()` (`normalizer.go:218-245`), read by the snapshot ticker (`live.go:436-439`, `:476-481`) which today shares the goroutine. Either a mutex in `Normalizer` or a snapshot copied out by the worker.

Acceptable under the repo's rules? Only with a design edit to `normalization.md` and a measurement showing decode is the bottleneck. The only latency the repo measures is the bbolt commit (`recorder.go:486-516`) and the capture queue (`raw-capture.md:118-120`); nothing measures decode. Adding a lock to reproduce, in a more complex way, what a single goroutine already gives is the opposite of "Small".

## 6. Question 5: live vs replay availability under a batching admit stage

Live availability is `Clock()` at `Admit` (`recorder.go:389-392`). A `Batcher` in front of `Admit` with `MaxWait: 20ms` moves every live stamp later by up to `MaxWait` plus queueing. Acceptance (2) excludes `available_time`, so the test does not break, and "the delay is recorded honestly" (DESIGN.md, Ordering section) tolerates transport delay. So batching alone is a tolerated degradation, not a break.

The break is F1, not batching: with `InOrder` a live trade's stamp is delayed by an unrelated slot's block fetch (bounded only by the two-minute budget, or unbounded if the block never comes), and a replay stamp is pushed from "the completing record's receipt time" to "the receipt time of whatever earlier-ordinal fact was released just before it". Both remain "stamped when serialized admission accepted the evidence" in letter and neither in spirit, because admission was made to wait for something the evidence did not depend on.

## 7. Question 6: would the maintainer adopt this, and what pays for itself

Against DESIGN.md:20 ("One well-known library per job, batching only after a measurement asks for it"), DESIGN.md:85 (GoBatch "added only if a measurement shows a batching win"), and `mvp-phases.md:22` ("Do not recreate the archived actor, hub, quota or accounting systems"): the `workflow` package is a keyed-task hub with dependency edges, a shared quota (`Pool` with priority), and accounting (`Stats`, `Observer`). It is the archived system with generics. No measurement in the repo asks for any of it. The answer is no.

Minimum subset that pays for itself today: **none**. In order of when something could:

1. Phase 5 Valkey flush (`mvp-phases.md:116`): `batch.Batcher[T]` with `MaxWait: 1s` *if* the ticker-based flush measures badly. This is the only place `Policy` is the right abstraction, and it needs nothing from section 5.
2. Admission-log group commit, only when `CommitLatency` shows fsync-bound admission, and implemented as a transactional `Memory` API, not a `Batcher` (F5). GoBatch is not on that path.
3. Never: `workflow` for the Helius scheduler (capture path, irreplaceable bytes, 780 lines of ruling tests, F3) or for the normalizer (F1, F2, Q4).

`Group` (4.6, request/reply) solves a shape ShitQuant does not have: nobody awaits a per-signature status; statuses are journaled and read later by the normalizer.

## 8. Question 7: misreads, with location

1. 6.2 line 594: the normalizer submits a `confirmed` fetch. The normalizer is network-free and reads records (`normalization.md:9`, `normalizer.go:251-287`). F2.
2. 6.2 / 5.5: admit tasks "keyed by occurrence ID". Occurrence ID contains the blockhash (`normalizer.go:478`), unknown at decode time.
3. 6.2: "`time_unknown` is `build` returning a typed error ... not a task failure the library retries". Per 5.3, a `build` error *fails* the task with `*DependencyError`; the consumer then counts a failure event, not a return value. Today it is `n.count(ReasonTimeUnknown)` at `normalizer.go:378`, `:420`.
4. 6.2: "a replay over the same capture releases the same tasks in the same order: acceptance (1) ... preserved by construction". Acceptance (1) holds today by construction (single goroutine). The proposal preserves determinism while changing the order (F1).
5. 6.1 line 548: `Policy{MaxItems: 256}` releases each item immediately under 4.1. F3a.
6. 6.1 lines 543-550: one `Retry` policy for all kinds makes skipped-slot codes terminal at confirmed commitment, contradicting `raw-capture.md:63` and `request.go:238-248`. F3b.
7. 6.1: "the 'which of these is unresolved' bookkeeping (`Stats` and `*Incomplete` at `Close`)". `Unresolved` has no production caller; inspection derives unresolved from the journal (`scheduler.go:153-154`, `:344-362`); `*Incomplete` lists only gate-blocked tasks and only at `Close`, which the capture cannot use (F3c).
8. 6.1: "the sliding-window rate limiter" disappears. It is replaced by a token bucket with different burst behaviour. F3f.
9. 6.1: "536 lines of state machine plus a 200 ms ticker loop". The ticker is in `request.go:76`, not in the 536; roughly half the 536 are comments. F4.
10. 6.3: "Nothing changes inside" the recorder, and group commit "reduced to one integer". `memory.go:515-553` persists one mutation per lock hold; group commit is a `Memory` API change. F5.
11. 5.4: `Gate.Wait` "is ... `Journal.Wait` ... offered once". `Journal.Wait` (`journal.go:345-373`) also returns on `Closed()` and on handle stop (`ErrJournalClosed`), which `reader.go:45-56` needs to yield `io.EOF`. A `uint64` watermark cannot express "closed" without advancing to a sentinel, i.e. manufacturing a value.
12. 4.2: "Close means drain ... what a `--duration` cap that expires normally needs". For the request loop the normal end is abandon-and-drain-results (`request.go:72-74`, `source.go:378-386`). F3c.
13. 2 (goal 5): "dedup by first receipt". ShitQuant's first-receipt rule is the recorder's (`recorder.go:343-349`) and the scheduler's is permanent (`scheduler.go:186-208`); the library's while-pending dedup (7) is neither.
14. 6.4: "The one-second candle flush is a `Batcher[CandleUpdate]`". `mvp-phases.md:116`: ticker first, "GoBatch only if the flush measurement asks for it". The paper-runner kind is speculation about an undesigned phase.
15. 5.8's attribution of "plain `select` among ready channels is not the priority mechanism" to "the archived design" could not be verified in the current tree or the archive branch's CLAUDE.md/DESIGN.md; treat as unverified.
16. Delivery plan step 4: "replace the Helius scheduler first (smallest blast radius, most state removed)". Largest blast radius (capture path), and the state mostly moves (F4).
17. Accurate claims, for balance: the per-attempt goroutine, unbuffered `results` channel and drain (`request.go:85-132`); Go 1.26 (`go.mod:3`); priority order confirmed → statuses → finalized (`scheduler.go:256-296`); two in flight, five per second (`scheduler.go:70-78`); the journal's generation-swap wait (`journal.go:338-343`); "the consumer hand-writes a scheduler and a pending-state machine".

---

## 9. Bottom line for the proposal author

- Drop section 6 as written. It is not a mapping of ShitQuant; it is a mapping of a fetch-oriented pipeline that ShitQuant deliberately is not (DESIGN.md topology: the normalizer reads the journal, never the socket).
- If a consumer-driven justification for `workflow` is wanted, it needs a primitive the proposal lacks: a keyed, complete-once, externally-completed promise with a "may never complete, forget it at the end" story, and an ordering rule keyed on *completion* record, not first dependency. With those, ask whether it is still smaller than the maps at `normalizer.go:83-104`.
- Keep `batch.Batcher` and `Policy`; they are the only parts with a plausible ShitQuant use (phase 5 flush), and only after that measurement.
- Fix the proposal's own examples (F3a, F3b) and resolve 5.7 vs 5.9 before any consumer reads it again.
