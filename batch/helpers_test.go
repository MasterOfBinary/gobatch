package batch

import (
	"errors"
	"strconv"
	"testing"
	"time"
)

func TestNextItem(t *testing.T) {
	t.Run("with item", func(t *testing.T) {
		ch := make(chan *Item[any])

		go func() {
			ch <- &Item[any]{
				id: 100,
			}
		}()

		ps := &PipelineStage[any, any]{
			Input: ch,
		}

		next := NextItem(ps, 200)
		if next.id != 100 {
			t.Errorf("next.id == %v, want 100", next.id)
		}
		if next.item != 200 {
			t.Errorf("next.item == %v, want 200", next.item)
		}
	})

	t.Run("closed channel", func(t *testing.T) {
		ch := make(chan *Item[any])
		close(ch)

		ps := &PipelineStage[any, any]{
			Input: ch,
		}

		if next := NextItem(ps, 200); next != nil {
			t.Errorf("next == %v, want nil", next)
		}
	})
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
