package batch

import "sync"

// Item holds a single item in the batch processing pipeline.
type Item[T any] struct {
	mu   sync.RWMutex
	id   uint64
	item T
}

// GetID returns a unique ID of the current item in the pipeline.
func (i *Item[T]) GetID() uint64 {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.id
}

// Get returns the item data.
func (i *Item[T]) Get() T {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.item
}

// Set sets the item data.
func (i *Item[T]) Set(item T) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.item = item
}
