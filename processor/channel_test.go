package processor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestChannel_Process(t *testing.T) {
	t.Run("writes data to output channel", func(t *testing.T) {
		out := make(chan interface{}, 3)
		processor := &Channel{Output: out}

		items := []*batch.Item{
			{ID: 1, Data: "a"},
			{ID: 2, Data: "b"},
			{ID: 3, Data: "c"},
		}

		ctx := context.Background()
		result, err := processor.Process(ctx, items)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(result) != len(items) {
			t.Errorf("expected %d items, got %d", len(items), len(result))
		}

		close(out)
		var collected []interface{}
		for v := range out {
			collected = append(collected, v)
		}

		if len(collected) != len(items) {
			t.Errorf("expected %d values, got %d", len(items), len(collected))
		}

		for i, v := range collected {
			if v != items[i].Data {
				t.Errorf("value %d: expected %v, got %v", i, items[i].Data, v)
			}
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		out := make(chan interface{})
		processor := &Channel{Output: out}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		items := []*batch.Item{{ID: 1, Data: "test"}}
		_, err := processor.Process(ctx, items)

		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}

		select {
		case <-time.After(50 * time.Millisecond):
			// output channel should be empty
		case v := <-out:
			t.Errorf("expected no value sent, got %v", v)
		}
	})

	t.Run("handles nil output channel", func(t *testing.T) {
		processor := &Channel{Output: nil}
		items := []*batch.Item{{ID: 1, Data: "data"}}
		ctx := context.Background()

		result, err := processor.Process(ctx, items)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(result) != 1 || result[0].Data != "data" {
			t.Errorf("items should be unchanged")
		}
	})

	t.Run("skips items with existing errors", func(t *testing.T) {
		out := make(chan interface{}, 2)
		processor := &Channel{Output: out}

		items := []*batch.Item{
			{ID: 1, Data: "a", Error: errors.New("fail")},
			{ID: 2, Data: "b"},
		}

		ctx := context.Background()
		_, err := processor.Process(ctx, items)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		close(out)

		var collected []interface{}
		for v := range out {
			collected = append(collected, v)
		}

		if len(collected) != 1 || collected[0] != "b" {
			t.Errorf("expected only 'b' in output, got %v", collected)
		}
	})

	t.Run("handles empty slice", func(t *testing.T) {
		out := make(chan interface{}, 1)
		processor := &Channel{Output: out}
		ctx := context.Background()

		result, err := processor.Process(ctx, []*batch.Item{})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(result) != 0 {
			t.Errorf("expected empty result, got %d", len(result))
		}

		// there should be nothing in the channel
		select {
		case v := <-out:
			t.Errorf("unexpected value %v", v)
		default:
		}
	})
}
