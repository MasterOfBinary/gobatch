package processor

import (
	"context"
	"sync"
	"testing"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestCollect_Process(t *testing.T) {
	t.Run("appends item data", func(t *testing.T) {
		c := &Collect{}
		items := []*batch.Item{
			{ID: 1, Data: "a"},
			{ID: 2, Data: "b"},
		}
		ctx := context.Background()
		_, err := c.Process(ctx, items)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(c.Data) != 2 {
			t.Fatalf("expected 2 items collected, got %d", len(c.Data))
		}
		if c.Data[0] != "a" || c.Data[1] != "b" {
			t.Fatalf("unexpected collected data: %v", c.Data)
		}
	})

	t.Run("safe for concurrent use", func(t *testing.T) {
		c := &Collect{}
		wg := sync.WaitGroup{}
		ctx := context.Background()
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				items := []*batch.Item{{ID: uint64(id), Data: id}}
				c.Process(ctx, items)
			}(i)
		}
		wg.Wait()
		if len(c.Data) != 5 {
			t.Fatalf("expected 5 items collected, got %d", len(c.Data))
		}
	})
}

func TestCollect_EmptySlice(t *testing.T) {
	c := &Collect{}
	ctx := context.Background()
	_, err := c.Process(ctx, []*batch.Item{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.Data) != 0 {
		t.Fatalf("expected no data collected, got %d", len(c.Data))
	}
}
