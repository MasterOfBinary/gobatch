package panicguard

import (
	"runtime"
	"strings"
	"testing"
)

// Deep recursion must retain bounded diagnostics and disclose truncation rather
// than allocating an unbounded traceback or silently claiming a complete one.
func TestPanicDiagnosticBound(t *testing.T) {
	var got *PanicError
	Run("test.deep", func() {
		recurseThroughDeliberatelyLongNamedBoundaryToExerciseTheRetainedDiagnosticByteLimitWithoutChangingRuntimeTracebackSettings(200)
	}, func(err *PanicError) { got = err })
	if got == nil || !got.Truncated || len(got.Stack()) != 16*1024 {
		t.Fatalf("missing bounded truncated diagnostic: %#v", got)
	}
	if strings.Contains(got.Error(), "credential") {
		t.Fatal("panic value leaked")
	}
}

//go:noinline
func recurseThroughDeliberatelyLongNamedBoundaryToExerciseTheRetainedDiagnosticByteLimitWithoutChangingRuntimeTracebackSettings(n int) {
	if n == 0 {
		panic("credential fixture")
	}
	recurseThroughDeliberatelyLongNamedBoundaryToExerciseTheRetainedDiagnosticByteLimitWithoutChangingRuntimeTracebackSettings(n - 1)
	runtime.KeepAlive(n)
}

// An abnormal goroutine return must report failure before owner cleanup even
// when Goexit does not resume Run's caller. Normal execution emits no failure.
func TestPanicGuardAbnormalReturnCleanup(t *testing.T) {
	reports := make(chan *PanicError, 1)
	done := make(chan struct{})
	go func() { defer close(done); Run("test.exit", runtime.Goexit, func(err *PanicError) { reports <- err }) }()
	<-done
	select {
	case err := <-reports:
		if err.Boundary != "test.exit" {
			t.Fatal(err)
		}
	default:
		t.Fatal("abnormal return reported success")
	}
	called := false
	Run("test.normal", func() { called = true }, func(err *PanicError) { t.Fatal(err) })
	if !called {
		t.Fatal("normal operation was skipped")
	}
}
