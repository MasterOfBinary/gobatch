package gobatch

import "sync"

// itemImpl is an implementation of the source.Item and processor.Item interfaces.
type itemImpl struct {
	mu   sync.RWMutex
	id   uint64
	item interface{}
}

// GetID implements the item.Item interface.
func (i *itemImpl) GetID() uint64 {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.id
}

// Get implements the item.Item interface.
func (i *itemImpl) Get() interface{} {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.item
}

// Set implements the item.Item interface.
func (i *itemImpl) Set(item interface{}) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.item = item
}
