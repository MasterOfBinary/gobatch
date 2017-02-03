package gobatch_test

import (
	"errors"
	"testing"

	"github.com/MasterOfBinary/gobatch/mocks"
)

func TestMust(t *testing.T) {
	batch := &mocks.Batch{}
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
		_ = Must(&mocks.Batch{}, errors.New("error"))
	}()

	if !panics {
		t.Error("Must(batch, err) doesn't panic")
	}
}
