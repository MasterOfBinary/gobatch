// Package panicguard provides the shared boundary for library-owned goroutines.
package panicguard

import "runtime"

const maxStackBytes = 16 * 1024

// PanicError reports a recovered panic without retaining or formatting its value.
// Stack diagnostics may contain sensitive paths and arguments; disclose them only
// through a protected diagnostic sink. Ordinary formatting exposes only Boundary.
type PanicError struct {
	Boundary  string
	Truncated bool
	stack     []byte
}

// Error returns safe boundary metadata, never the panic value or stack.
func (e *PanicError) Error() string { return "panic in " + e.Boundary }

// Stack returns a defensive copy of at most 16 KiB of diagnostic stack data.
func (e *PanicError) Stack() []byte { return append([]byte(nil), e.stack...) }

// Run calls fn on the current goroutine and reports abnormal return there. The
// normal-return sentinel also detects panic(nil) under Go 1.18 semantics. report
// must not panic and may apply backpressure. The owner's cleanup belongs in outer
// defers, so runtime.Goexit also reports failure and runs that cleanup.
func Run(boundary string, fn func(), report func(*PanicError)) {
	returned := false
	defer func() {
		if !returned {
			_ = recover() // Deliberately discard the value; even its Error may panic.
			stack := make([]byte, maxStackBytes)
			n := runtime.Stack(stack, false)
			report(&PanicError{Boundary: boundary, Truncated: n == len(stack), stack: stack[:n]})
		}
	}()
	fn()
	returned = true
}
