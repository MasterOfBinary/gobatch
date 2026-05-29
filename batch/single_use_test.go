package batch_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/MasterOfBinary/gobatch/batch"
)

// TestGo_SecondCallReturnsErrBatchUsed pins the single-use contract: a Batch may
// run exactly once. A second Go() on the same Batch — even after the first run
// has fully completed — returns ErrBatchUsed instead of starting another run.
// Callers that want to run again must create a fresh Batch with New.
func TestGo_SecondCallReturnsErrBatchUsed(t *testing.T) {
	b := New[any](NewConstantConfig(&ConfigValues{MinItems: 1, MaxItems: 5}))

	errs, err := b.Go(context.Background(), &testSource{Items: []any{1, 2, 3}}, &countProcessor{})
	if err != nil {
		t.Fatalf("first Go returned unexpected error: %v", err)
	}
	IgnoreErrors(errs)
	<-b.Done()

	errs2, err2 := b.Go(context.Background(), &testSource{Items: []any{4, 5, 6}}, &countProcessor{})
	if !errors.Is(err2, ErrBatchUsed) {
		t.Fatalf("second Go: got error %v, want ErrBatchUsed", err2)
	}
	// A rejected Go must still return a non-nil, already-closed channel so a
	// caller that ranges over it without checking err does not block forever.
	if errs2 == nil {
		t.Fatal("second Go returned a nil error channel; want a closed, drainable channel")
	}
	for range errs2 {
		t.Fatal("error channel from a rejected Go should be empty")
	}
}

// TestGo_NilSourceReturnsErrNilSource verifies that a nil Source is reported via
// the returned error rather than being smuggled onto the pipeline error channel.
func TestGo_NilSourceReturnsErrNilSource(t *testing.T) {
	b := New[any](NewConstantConfig(&ConfigValues{}))

	errs, err := b.Go(context.Background(), nil)
	if !errors.Is(err, ErrNilSource) {
		t.Fatalf("Go(nil): got error %v, want ErrNilSource", err)
	}
	if errs == nil {
		t.Fatal("Go(nil) returned a nil error channel; want a closed, drainable channel")
	}
	for range errs {
		t.Fatal("error channel from a rejected Go should be empty")
	}
}

// TestGo_SuccessReturnsNilError verifies the happy path returns a nil start error
// alongside the live pipeline error channel.
func TestGo_SuccessReturnsNilError(t *testing.T) {
	b := New[any](NewConstantConfig(&ConfigValues{MinItems: 1, MaxItems: 5}))

	errs, err := b.Go(context.Background(), &testSource{Items: []any{1, 2, 3}}, &countProcessor{})
	if err != nil {
		t.Fatalf("Go returned unexpected start error: %v", err)
	}
	IgnoreErrors(errs)
	<-b.Done()
}
