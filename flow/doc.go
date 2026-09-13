// Package flow composes finite context-aware operations using Step, Execute,
// Sequence and Parallel. Serial work stops at the first error. Parallel starts
// every declared branch once, joins all branches even after failure or panic,
// and returns the first error in declaration order, not completion order.
// Empty compositions succeed; explicit nil steps return ErrNilStep.
//
// Compositions copy the declaration slice. Callers own captured state: parallel
// inputs must be immutable and branches must write separate outputs, merged only
// after joining. Reuse is safe only when the captured callbacks/state permit it.
// Branch count and callback work must be finite and explicitly caller-bounded.
// There is no dynamic scheduler, retry, persistence or domain policy here.
//
// The parent context is passed unchanged, even when already canceled. Callbacks
// decide how to observe cancellation and report it; sibling errors never cancel
// each other. No timeout abandons started goroutines. A noncooperative callback
// keeps its invocation open until it finishes. Callback-owned children require
// their own recovery and join; recovering the callback cannot guard its children.
//
// Each callback runs inside flow's panic boundary. PanicError covers legacy
// panic(nil) handling and bounded protected diagnostics. Error formatting omits
// both panic value and stack.
// Recovery is not rollback and cannot handle fatal runtime failures. Goexit in
// a Parallel branch reports failure and joins through deferred cleanup. Execute
// and Sequence run on the caller's goroutine; Goexit there unwinds that goroutine
// and cannot be turned into a normal return to its caller.
//
// Parallel returns only its first declaration-ordered failure. Applications that
// need every branch failure must retain them in independent bounded result slots
// through their own wrappers, including recovery for their own diagnostic needs.
//
// A step containing a Batch must check startup failure, drain its error channel
// and await completion before returning. RunBatchAndWait does this for finite,
// caller-bounded workloads and error counts. Starting Batch.Go alone is not a
// completed dependency. Use a fresh Batch on every step invocation.
package flow
