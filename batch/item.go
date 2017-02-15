package batch

import "sync"

// Item holds a single item in the batch processing pipeline.
type Item struct {
	mu   sync.RWMutex
	id   uint64
	item interface{}
}

// GetID returns a unique ID of the current item in the pipeline.
func (i *Item) GetID() uint64 {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.id
}

// Get returns the item data.
func (i *Item) Get() interface{} {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.item
}

// Set sets the item data.
func (i *Item) Set(item interface{}) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.item = item
}
