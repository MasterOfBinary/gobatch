package flow

import (
	"context"
	"sync"

	"github.com/MasterOfBinary/gobatch/internal/panicguard"
)

// Step is a finite operation. It must observe cancellation cooperatively and
// return only after its own child work has finished. Inputs shared by parallel
// steps must be immutable; each branch must own its outputs until the join.
type Step func(context.Context) error

// Execute runs steps on the caller's goroutine in declaration order, stopping at
// the first error (including a recovered panic or ErrNilStep). Empty work succeeds.
// The context is passed unchanged: cancellation is the step's responsibility.
func Execute(ctx context.Context, steps ...Step) error {
	for _, step := range steps {
		var err error
		invoke(ctx, step, &err)
		if err != nil {
			return err
		}
	}
	return nil
}

// Sequence captures a copy of the declared steps and returns their serial
// composition. Each invocation runs Execute with the supplied context.
func Sequence(steps ...Step) Step {
	declared := append([]Step(nil), steps...)
	return func(ctx context.Context) error { return Execute(ctx, declared...) }
}

// Parallel captures a fixed set of branches, starts every branch once per call,
// and joins all of them before returning the first error in declaration order.
// Branch errors and panics do not cancel siblings. Parent cancellation is passed
// unchanged and cannot force a noncooperative branch to finish. Empty work succeeds.
// The caller must bound branch count; this function has no worker pool or retries.
func Parallel(steps ...Step) Step {
	declared := append([]Step(nil), steps...)
	return func(ctx context.Context) error {
		results := make([]error, len(declared))
		var wg sync.WaitGroup
		wg.Add(len(declared))
		for i, step := range declared {
			go func(i int, step Step) {
				defer wg.Done()
				invoke(ctx, step, &results[i])
			}(i, step)
		}
		wg.Wait()
		for _, err := range results {
			if err != nil {
				return err
			}
		}
		return nil
	}
}

// Write through the owned slot inside the guard: Goexit runs deferred recovery
// and owner cleanup but never returns to a caller-side result assignment.
func invoke(ctx context.Context, step Step, result *error) {
	panicguard.Run("flow.step", func() {
		if step == nil {
			*result = ErrNilStep
			return
		}
		*result = step(ctx)
	}, func(err *panicguard.PanicError) { *result = err })
}
