package processor

import (
	"context"
	"sync"

	"github.com/MasterOfBinary/gobatch/batch"
)

// Collect is a processor that gathers item data into the Data slice.
//
// It is useful for capturing results from a processing pipeline.
// Collect is safe for concurrent use because Process may be invoked
// by multiple goroutines.
type Collect struct {
	mu   sync.Mutex
	Data []interface{}
}

// Process appends each item's Data to the Data slice.
func (c *Collect) Process(_ context.Context, items []*batch.Item) ([]*batch.Item, error) {
	if len(items) == 0 {
		return items, nil
	}

	c.mu.Lock()
	for _, item := range items {
		c.Data = append(c.Data, item.Data)
	}
	c.mu.Unlock()

	return items, nil
}
