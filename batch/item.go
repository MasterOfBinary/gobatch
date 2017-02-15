package batch

import "sync"

// Item holds a single item in the batch processing pipeline.
type Item struct {
	mu   sync.RWMutex
	id   uint64
	item interface{}
}

// GetID implements the item.Item interface.
func (i *Item) GetID() uint64 {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.id
}

// Get implements the item.Item interface.
func (i *Item) Get() interface{} {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.item
}

// Set implements the item.Item interface.
func (i *Item) Set(item interface{}) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.item = item
}
