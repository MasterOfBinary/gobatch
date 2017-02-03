package gobatch

import (
	"errors"
	"strconv"
	"testing"
	"time"
)

func TestMust(t *testing.T) {
	batch := &MockBatch{}
	if Must(batch, nil) != batch {
		t.Error("Must(batch, nil) != batch")
	}

	var panics bool
	func() {
		defer func() {
			if p := recover(); p != nil {
				panics = true
			}
		}()
		_ = Must(&MockBatch{}, errors.New("error"))
	}()

	if !panics {
		t.Error("Must(batch, err) doesn't panic")
	}
}

func TestIgnoreErrors(t *testing.T) {
	done := make(chan struct{})
	errs := make(chan error)

	go func() {
		for i := 0; i < 10; i++ {
			errs <- errors.New("error " + strconv.Itoa(i))
		}
		close(errs)
		close(done)
	}()

	IgnoreErrors(errs)

	// Make sure the first goroutine is able to complete. Otherwise it
	// wasn't able to write to the error channel
	select {
	case <-done:
		break
	case <-time.After(time.Second):
		t.Error("Writing goroutine didn't complete")
	}
}
